// Command imgbench measures how fast this machine processes a folder of photos.
//
// For each combination of preview format and worker count it runs the CPU and
// disk part of the ingest pipeline on every photo: hash the original, generate
// the previews, and write them to disk. EXIF extraction and database writes
// are not included.
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/denuser/fuchsia-photos/backend/internal/media"
)

var extensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".heic": true, ".heif": true,
}

// progressInterval is how often a run reports how far it has got.
const progressInterval = 5 * time.Second

type result struct {
	class       string
	duration    time.Duration
	viewerBytes int
	gridBytes   int
	err         error
}

func main() {
	in := flag.String("in", "", "folder with photos, read recursively (required)")
	out := flag.String("out", "", "folder for the previews, on the disk being tested (required)")
	workers := flag.String("workers", "1,2,3", "comma-separated worker counts")
	formats := flag.String("formats", "webp,jpeg", "comma-separated preview formats")
	limit := flag.Int("limit", 0, "maximum number of photos, 0 means all")
	flag.Parse()

	if *in == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	workerCounts, err := parseWorkers(*workers)
	if err != nil {
		log.Fatal(err)
	}

	files, err := findPhotos(*in, *limit)
	if err != nil {
		log.Fatal(err)
	}
	if len(files) == 0 {
		log.Fatalf("no photos found in %s", *in)
	}

	if err := media.Startup(); err != nil {
		log.Fatal(err)
	}
	defer media.Shutdown()

	fmt.Printf("%d photos from %s, previews written to %s\n\n", len(files), *in, *out)
	fmt.Println("| Format | Workers | Type | Photos | Failed | Median | p95 | Viewer avg | Grid avg |")
	fmt.Println("|---|---|---|---:|---:|---:|---:|---:|---:|")

	formatList := strings.Split(*formats, ",")
	runs := len(formatList) * len(workerCounts)
	runNumber := 0

	failed := 0
	for _, format := range formatList {
		for _, n := range workerCounts {
			runNumber++
			label := fmt.Sprintf("run %d/%d, %s, workers %d", runNumber, runs, format, n)
			start := time.Now()
			results, err := run(files, *out, media.Format(format), n, label)
			if err != nil {
				log.Fatal(err)
			}
			elapsed := time.Since(start)

			failed += report(format, n, results)
			ok := len(results) - countFailed(results)
			fmt.Printf("| %s | %d | **all** | %d | %d | **%.1f photos/s** | %s total | | |\n",
				format, n, len(results), countFailed(results),
				float64(ok)/elapsed.Seconds(), elapsed.Round(time.Millisecond))
		}
	}

	fmt.Printf("\nPeak memory: %s\n", formatBytes(peakRSS()))
	fmt.Println("Originals may come from the page cache after the first row. Clear it between runs to measure a cold disk.")
	if failed > 0 {
		os.Exit(1)
	}
}

func parseWorkers(s string) ([]int, error) {
	var counts []int
	for part := range strings.SplitSeq(s, ",") {
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid worker count %q", part)
		}
		counts = append(counts, n)
	}
	return counts, nil
}

func findPhotos(root string, limit int) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && extensions[strings.ToLower(filepath.Ext(path))] {
			files = append(files, path)
		}
		return nil
	})
	if limit > 0 && len(files) > limit {
		files = files[:limit]
	}
	return files, err
}

func run(files []string, out string, format media.Format, workers int, label string) ([]result, error) {
	dir, err := os.MkdirTemp(out, "imgbench-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	jobs := make(chan int)
	results := make([]result, len(files))

	prog := &progress{label: label, total: len(files), start: time.Now()}
	defer prog.reportEvery(progressInterval)()

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for i := range jobs {
				results[i] = process(files[i], filepath.Join(dir, strconv.Itoa(i)), format)
				if results[i].err != nil {
					log.Printf("%s: %v", files[i], results[i].err)
					prog.failed.Add(1)
				}
				prog.done.Add(1)
			}
		})
	}
	for i := range files {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results, nil
}

func process(path, outPrefix string, format media.Format) result {
	res := result{class: strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))}
	if res.class == "jpeg" {
		res.class = "jpg"
	}

	start := time.Now()
	if res.err = hashFile(path); res.err != nil {
		return res
	}
	settings := media.DefaultSettings()
	settings.Format = format
	previews, err := media.Render(path, settings)
	if err != nil {
		res.err = err
		return res
	}
	if res.err = os.WriteFile(outPrefix+"-viewer", previews.Viewer.Data, 0o600); res.err != nil {
		return res
	}
	if res.err = os.WriteFile(outPrefix+"-grid", previews.Grid.Data, 0o600); res.err != nil {
		return res
	}
	res.duration = time.Since(start)
	res.viewerBytes = len(previews.Viewer.Data)
	res.gridBytes = len(previews.Grid.Data)
	return res
}

func hashFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(sha256.New(), f)
	return err
}

// progress tracks one run. It is reported through the log, on stderr, so that
// stdout holds only the results table.
type progress struct {
	label  string
	total  int
	start  time.Time
	done   atomic.Int64
	failed atomic.Int64
}

// reportEvery logs the progress at each interval until the returned function is
// called. The first line appears only after an interval, so a short run stays quiet.
func (p *progress) reportEvery(interval time.Duration) (stop func()) {
	finished := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				log.Print(p.line(time.Now()))
			case <-finished:
				return
			}
		}
	})
	return func() {
		close(finished)
		wg.Wait()
	}
}

func (p *progress) line(now time.Time) string {
	done := int(p.done.Load())
	line := fmt.Sprintf("%s: %d/%d photos, %d failed", p.label, done, p.total, p.failed.Load())

	elapsed := now.Sub(p.start)
	if done == 0 || elapsed <= 0 {
		return line
	}
	left := time.Duration(float64(elapsed) * float64(p.total-done) / float64(done))
	return fmt.Sprintf("%s, %.1f photos/s, %s left",
		line, float64(done)/elapsed.Seconds(), left.Round(time.Second))
}

// report prints one table row per file type and returns the number of failures.
func report(format string, workers int, results []result) int {
	byClass := map[string][]result{}
	for _, r := range results {
		byClass[r.class] = append(byClass[r.class], r)
	}

	failed := 0
	for _, class := range slices.Sorted(maps.Keys(byClass)) {
		group := byClass[class]
		var durations []time.Duration
		var viewer, grid int
		for _, r := range group {
			if r.err != nil {
				failed++
				continue
			}
			durations = append(durations, r.duration)
			viewer += r.viewerBytes
			grid += r.gridBytes
		}
		slices.Sort(durations)

		ok := len(durations)
		if ok == 0 {
			fmt.Printf("| %s | %d | %s | %d | %d | | | | |\n", format, workers, class, len(group), len(group))
			continue
		}
		fmt.Printf("| %s | %d | %s | %d | %d | %s | %s | %s | %s |\n",
			format, workers, class, len(group), len(group)-ok,
			durations[ok/2].Round(time.Millisecond),
			durations[(ok*95)/100].Round(time.Millisecond),
			formatBytes(int64(viewer/ok)), formatBytes(int64(grid/ok)))
	}
	return failed
}

func countFailed(results []result) int {
	n := 0
	for _, r := range results {
		if r.err != nil {
			n++
		}
	}
	return n
}

// peakRSS returns the peak memory of the process. Ru_maxrss is in kilobytes on
// Linux, which is where the benchmark runs.
func peakRSS() int64 {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0
	}
	return int64(usage.Maxrss) * 1024
}

func formatBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%d KB", n/(1<<10))
	}
}
