package main

import "strings"

type Lang struct {
	Code string
	Name string
}

// Languages supported by DeepL as source and target.
var languages = []Lang{
	{"ar", "Arabic"},
	{"bg", "Bulgarian"},
	{"cs", "Czech"},
	{"da", "Danish"},
	{"de", "German"},
	{"el", "Greek"},
	{"en", "English"},
	{"es", "Spanish"},
	{"et", "Estonian"},
	{"fi", "Finnish"},
	{"fr", "French"},
	{"hu", "Hungarian"},
	{"id", "Indonesian"},
	{"it", "Italian"},
	{"ja", "Japanese"},
	{"ko", "Korean"},
	{"lt", "Lithuanian"},
	{"lv", "Latvian"},
	{"nb", "Norwegian"},
	{"nl", "Dutch"},
	{"pl", "Polish"},
	{"pt", "Portuguese"},
	{"ro", "Romanian"},
	{"ru", "Russian"},
	{"sk", "Slovak"},
	{"sl", "Slovenian"},
	{"sv", "Swedish"},
	{"tr", "Turkish"},
	{"uk", "Ukrainian"},
	{"zh", "Chinese"},
}

// pickerLangs lists the configured pair first, then everything else.
func pickerLangs(home, foreign string) []Lang {
	var first, rest []Lang
	for _, l := range languages {
		if l.Code == home || l.Code == foreign {
			first = append(first, l)
		} else {
			rest = append(rest, l)
		}
	}
	return append(first, rest...)
}

func direction(from, to string) string {
	return from + "⇒" + to
}

func upper(code string) string { return strings.ToUpper(code) }

func langName(code string) string {
	for _, l := range languages {
		if l.Code == code {
			return l.Name
		}
	}
	return code
}
