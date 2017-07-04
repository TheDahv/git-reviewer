package gitreviewers

import (
	"testing"
)

func TestDefaultIgnoreExtensions(t *testing.T) {
	// All defaults
	if considerExt("myfile.svg", &ContributionCounter{}) {
		t.Error("Expected SVG files to be ignored by default")
	}

	if considerExt("myfile.json", &ContributionCounter{}) {
		t.Error("Expected JSON files to be ignored by default")
	}

	if considerExt("myfile.nock", &ContributionCounter{}) {
		t.Error("Expected NOCK files to be ignored by default")
	}

	if considerExt("myfile.xml", &ContributionCounter{}) {
		t.Error("Expected XML files to be ignored by default")
	}

	// Defaults in addition to extra extensions
	opts := &ContributionCounter{IgnoredExtensions: []string{"coffee"}}
	if considerExt("myfile.coffee", opts) {
		t.Error("Expected coffee files to be explicitly ignored")
	}

	if considerExt("myfile.json", opts) {
		t.Error("Expected JSON files to be ignored when other ignores defined")
	}
}

func TestChooseTopN(t *testing.T) {
	var (
		stats      Stats
		srcSize    = 10000
		outputSize = 5
	)

	for i := 0; i < srcSize; i++ {
		stats = append(stats, &Stat{"", float64(i)})
	}

	actual := chooseTopN(outputSize, stats)
	if l := len(actual); l != outputSize {
		t.Errorf("chooseTopN result had %d elements, expected %d\n",
			l, srcSize)
	}

	for i := 0; i < outputSize; i++ {
		expected := (float64(srcSize-1) - float64(i))
		if p := actual[i].Percentage; p != expected {
			t.Errorf("Percentage at index %d is %f, expected %f\n",
				i, p, expected)
		}
	}

}
