package ml

import (
	"math"
	"testing"
)

// unit returns a unit vector at the given angle in the first two dimensions;
// the cosine distance between angles a and b is 1-cos(a-b).
func unit(angle float64) []float32 {
	v := make([]float32, 128)
	v[0], v[1] = float32(math.Cos(angle)), float32(math.Sin(angle))
	return v
}

// angleAt returns the angle whose cosine distance from 0 is d.
func angleAt(d float64) float64 { return math.Acos(1 - d) }

func TestGroupJoinsAPersonWhoseFacesDiffer(t *testing.T) {
	// Four faces of one person, each 0.4 from every other. The old rule split
	// them: two groups within its margin of each other started a third.
	a := math.Sqrt(0.6)
	b := math.Sqrt(1 - 0.6)
	faces := []GroupFace{}
	for i := range 4 {
		v := make([]float32, 128)
		v[0], v[i+1] = float32(a), float32(b)
		faces = append(faces, GroupFace{ID: int64(i + 1), Photo: int64(i + 1), Vector: v, Eligible: true})
	}
	Group(faces)
	for _, f := range faces {
		if f.Group != faces[0].Group {
			t.Fatalf("face %d in group %d, want %d", f.ID, f.Group, faces[0].Group)
		}
	}
}

func TestGroupKeepsDistantPeopleApart(t *testing.T) {
	far := angleAt(0.8)
	faces := []GroupFace{
		{ID: 1, Photo: 1, Vector: unit(0), Eligible: true},
		{ID: 2, Photo: 2, Vector: unit(far), Eligible: true},
		{ID: 3, Photo: 3, Vector: unit(0.01), Eligible: true},
	}
	Group(faces)
	if faces[0].Group == faces[1].Group || faces[2].Group != faces[0].Group {
		t.Fatalf("groups %d %d %d", faces[0].Group, faces[1].Group, faces[2].Group)
	}
}

func TestGroupSkipsGroupsWithAFaceOfTheSamePhoto(t *testing.T) {
	faces := []GroupFace{
		{ID: 1, Photo: 1, Vector: unit(0), Eligible: true},
		{ID: 2, Photo: 1, Vector: unit(0.01), Eligible: true},
	}
	Group(faces)
	if faces[0].Group == faces[1].Group {
		t.Fatal("two faces of one photo share a group")
	}
}

func TestGroupLeavesOutIneligibleFacesAndKeepsFixedOnes(t *testing.T) {
	faces := []GroupFace{
		// Placed by hand into group 7, far from the rest: it stays there.
		{ID: 1, Photo: 1, Vector: unit(angleAt(0.9)), Group: 7, Fixed: true},
		// Ignored by hand: no group, and it does not anchor anything.
		{ID: 2, Photo: 2, Vector: unit(0), Fixed: true},
		{ID: 3, Photo: 3, Vector: unit(0), Group: 9, Eligible: false},
		{ID: 4, Photo: 4, Vector: unit(angleAt(0.9) + 0.01), Group: 3, Eligible: true},
	}
	Group(faces)
	if faces[0].Group != 7 || faces[1].Group != 0 {
		t.Fatalf("fixed faces moved: %d %d", faces[0].Group, faces[1].Group)
	}
	if faces[2].Group != 0 {
		t.Fatalf("left-out face in group %d", faces[2].Group)
	}
	if faces[3].Group != 7 {
		t.Fatalf("face near the placed one in group %d, want 7", faces[3].Group)
	}
}

func TestGroupableTreatsUnscoredFacesAsFirstPass(t *testing.T) {
	low, high := float32(0.7), float32(0.8)
	cases := []struct {
		retry bool
		score *float32
		want  bool
	}{
		{false, &low, true},
		{true, &low, false},
		{true, &high, true},
		{true, nil, true},
	}
	for _, c := range cases {
		if got := Groupable(c.retry, c.score, 0.75); got != c.want {
			t.Errorf("Groupable(%v, %v) = %v", c.retry, c.score, got)
		}
	}
}

func TestParseVector(t *testing.T) {
	v, err := parseVector("[0.5,-1,0.25]")
	if err != nil || len(v) != 3 || v[0] != 0.5 || v[1] != -1 || v[2] != 0.25 {
		t.Fatalf("parseVector = %v, %v", v, err)
	}
}
