package gitreviewers

import (
	"strings"
	"testing"
)

type mapping struct {
	From, To string
}

func TestParseMailmap(t *testing.T) {
	cases := []struct {
		Input    string
		Mappings []mapping
	}{
		{
			Input: `# A comment followed by a blank line

Abraham Lincoln <abe@git-reviewer.com>
<abe@git-reviewer.com> <abe@gmail.com>
George Washington <george@git-reviewer.com> <george@gmail.com>
George Washington <george@git-reviewer.com>  G-Money Washington <george@gmail.com>
`,
			Mappings: []mapping{
				{"Abraham Lincoln", "Abraham Lincoln"},
				{"abe@gmail.com", "abe@git-reviewer.com"},
				{"george@git-reviewer.com", "george@git-reviewer.com"},
				{"george@gmail.com", "george@git-reviewer.com"},
				{"George Washington", "George Washington"},
				{"G-Money Washington", "George Washington"},
			},
		},
	}

	for _, c := range cases {
		mm := make(mailmap)
		rdr := strings.NewReader(c.Input)

		readMailmapFromSource(mm, rdr)

		for _, m := range c.Mappings {
			if actual, ok := mm[m.From]; !ok {
				t.Errorf("Didn't find '%s' in the mailmap\n", m.From)
			} else if actual != m.To {
				t.Errorf("Mapped '%s' to '%s', but expected '%s'\n",
					m.From, actual, m.To)
			}
		}
	}
}

func TestParseMailmapLine(t *testing.T) {
	cases := []struct {
		Input, Name, Email string
		Offset, Read       int
	}{
		// Simple unaliased mapping
		{
			"Abraham Lincoln <abe@git-reviewer.com>",
			"Abraham Lincoln",
			"abe@git-reviewer.com",
			0,
			38,
		},
		// Only mapping email alias
		{
			"<abe@git-reviewer.com> <abe@gmail.com>",
			"",
			"abe@git-reviewer.com",
			0,
			22,
		},
		{
			"<abe@git-reviewer.com> <abe@gmail.com>",
			"",
			"abe@gmail.com",
			22,
			38,
		},
		// Canonical name/email, only email alias
		{
			"George Washington <george@git-reviewer.com> <george@gmail.com>",
			"George Washington",
			"george@git-reviewer.com",
			0,
			43,
		},
		// Full name/email mapping
		{
			"George Washington <george@git-reviewer.com>  G-Money Washington <george@gmail.com>",
			"George Washington",
			"george@git-reviewer.com",
			0,
			43,
		},
		{
			"George Washington <george@git-reviewer.com>  G-Money Washington <george@gmail.com>",
			"G-Money Washington",
			"george@gmail.com",
			43,
			82,
		},
	}

	for _, c := range cases {
		name, email, read := parseMailmapLine([]byte(c.Input), c.Offset)

		if name != c.Name {
			t.Errorf("Got name '%s', expected '%s'\n", name, c.Name)
		}

		if email != c.Email {
			t.Errorf("Got email '%s', expected '%s'\n", email, c.Email)
		}

		if read != c.Read {
			t.Errorf("Got num read '%d', expected '%d'\n", read, c.Read)
		}
	}
}
