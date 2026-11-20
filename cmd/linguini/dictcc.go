package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// dict.cc offers its dictionary as a tab-separated download (personal use):
//
//	Gift {n}	poison	noun	[chem.]
//
// Scanning the ~70 MB file costs ~40 ms, so Linguini builds a sorted index
// once (rebuilt when the file changes) and binary-searches it: ~1 ms.
//
// Index layout: magic, source size, source mtime, record count (int64 each),
// then count int64 offsets of the records, then the records sorted by key:
// uint16 key length, key, int64 offset of the line in the source.
// A key is the column digit ("0"/"1") followed by the normalised term.

const dictMagic = "LINGUINI-DICTCC1"

type dictIndex struct {
	src, idx *os.File
	count    int64
	langs    []string
}

var (
	dictOnce   sync.Once
	dictLoaded *dictIndex
	dictErr    error
)

func openDict(cfg *Config) (*dictIndex, error) {
	dictOnce.Do(func() {
		dictLoaded, dictErr = loadDict(expandHome(cfg.DictCC.File), cfg.DictCC.Langs)
	})
	return dictLoaded, dictErr
}

func loadDict(path string, langs []string) (*dictIndex, error) {
	src, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%s missing: %w", path, errUnsupported)
	}
	if err != nil {
		return nil, err
	}
	st, err := src.Stat()
	if err != nil {
		return nil, err
	}
	idxPath := filepath.Join(xdgDir("XDG_CACHE_HOME", ".cache"), "linguini",
		filepath.Base(path)+".idx")
	d, err := openIndex(idxPath, st)
	if err != nil {
		if err := buildIndex(path, idxPath, langs, st); err != nil {
			return nil, fmt.Errorf("building index: %w", err)
		}
		if d, err = openIndex(idxPath, st); err != nil {
			return nil, err
		}
	}
	d.src, d.langs = src, langs
	return d, nil
}

func openIndex(path string, src os.FileInfo) (*dictIndex, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	head := make([]byte, len(dictMagic)+24)
	if _, err := f.ReadAt(head, 0); err != nil {
		f.Close()
		return nil, err
	}
	n := head[len(dictMagic):]
	if string(head[:len(dictMagic)]) != dictMagic ||
		int64(binary.LittleEndian.Uint64(n)) != src.Size() ||
		int64(binary.LittleEndian.Uint64(n[8:])) != src.ModTime().UnixNano() {
		f.Close()
		return nil, errors.New("stale index")
	}
	return &dictIndex{idx: f, count: int64(binary.LittleEndian.Uint64(n[16:]))}, nil
}

func buildIndex(srcPath, idxPath string, langs []string, st os.FileInfo) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	type record struct {
		key string
		off int64
	}
	var recs []record
	for off := 0; off < len(data); {
		end := bytes.IndexByte(data[off:], '\n')
		if end < 0 {
			end = len(data) - off
		}
		line := string(data[off : off+end])
		if line != "" && line[0] != '#' {
			fields := strings.Split(line, "\t")
			for col := 0; col < 2 && col < len(fields); col++ {
				if k := dictKey(fields[col], langs[col]); k != "" && len(k) < 1<<16-1 {
					recs = append(recs, record{string(rune('0'+col)) + k, int64(off)})
				}
			}
		}
		off += end + 1
	}
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].key != recs[j].key {
			return recs[i].key < recs[j].key
		}
		return recs[i].off < recs[j].off
	})

	if err := os.MkdirAll(filepath.Dir(idxPath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(idxPath), ".dictcc-*.idx")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	w := bufio.NewWriter(tmp)
	put := func(v int64) { binary.Write(w, binary.LittleEndian, v) }
	w.WriteString(dictMagic)
	put(st.Size())
	put(st.ModTime().UnixNano())
	put(int64(len(recs)))
	pos := int64(len(dictMagic)+24) + 8*int64(len(recs))
	for _, r := range recs {
		put(pos)
		pos += 2 + int64(len(r.key)) + 8
	}
	for _, r := range recs {
		binary.Write(w, binary.LittleEndian, uint16(len(r.key)))
		w.WriteString(r.key)
		put(r.off)
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), idxPath)
}

// record reads record i: its key and the source line offset.
func (d *dictIndex) record(i int64) (string, int64, error) {
	var b [8]byte
	if _, err := d.idx.ReadAt(b[:], int64(len(dictMagic)+24)+8*i); err != nil {
		return "", 0, err
	}
	pos := int64(binary.LittleEndian.Uint64(b[:]))
	if _, err := d.idx.ReadAt(b[:2], pos); err != nil {
		return "", 0, err
	}
	buf := make([]byte, int(binary.LittleEndian.Uint16(b[:2]))+8)
	if _, err := d.idx.ReadAt(buf, pos+2); err != nil {
		return "", 0, err
	}
	n := len(buf) - 8
	return string(buf[:n]), int64(binary.LittleEndian.Uint64(buf[n:])), nil
}

// lookup returns up to max source lines whose column col matches term.
func (d *dictIndex) lookup(col int, term string, max int) ([][]string, error) {
	target := string(rune('0'+col)) + dictKey(term, d.langs[col])
	lo, hi := int64(0), d.count
	for lo < hi {
		mid := (lo + hi) / 2
		k, _, err := d.record(mid)
		if err != nil {
			return nil, err
		}
		if k < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	var lines [][]string
	for i := lo; i < d.count && len(lines) < max; i++ {
		k, off, err := d.record(i)
		if err != nil {
			return nil, err
		}
		if k != target {
			break
		}
		line, err := d.line(off)
		if err != nil {
			return nil, err
		}
		lines = append(lines, strings.Split(line, "\t"))
	}
	return lines, nil
}

func (d *dictIndex) line(off int64) (string, error) {
	buf := make([]byte, 2048)
	n, err := d.src.ReadAt(buf, off)
	if err != nil && err != io.EOF {
		return "", err
	}
	line, _, _ := bytes.Cut(buf[:n], []byte("\n"))
	return string(line), nil
}

func (d *dictIndex) column(lang string) int {
	for i, l := range d.langs {
		if l == lang {
			return i
		}
	}
	return -1
}

func dictcc(ctx context.Context, cfg *Config, text, from, to string) ([]Entry, error) {
	d, err := openDict(cfg)
	if err != nil {
		return nil, err
	}
	src, dst := d.column(from), d.column(to)
	if src < 0 || dst < 0 || src == dst {
		return nil, fmt.Errorf("dictionary is %s: %w", strings.Join(d.langs, "-"), errUnsupported)
	}
	lines, err := d.lookup(src, text, cfg.DictCC.MaxResults)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	index := map[string]int{}
	for _, f := range lines {
		if len(f) <= dst {
			continue
		}
		term, annotations := splitTerm(f[dst])
		var note []string
		if len(f) > 2 && f[2] != "" {
			note = append(note, f[2])
		}
		note = append(note, annotations...)
		if len(f) > 3 && f[3] != "" {
			note = append(note, "· "+f[3])
		}
		if i, ok := index[term]; ok {
			// Same translation in another subject area: merge the notes.
			if len(f) > 3 && f[3] != "" && !strings.Contains(entries[i].Note, f[3]) {
				entries[i].Note += " " + f[3]
			}
			continue
		}
		index[term] = len(entries)
		entries = append(entries, Entry{Text: term, Note: strings.Join(note, " ")})
	}
	return entries, nil
}

// dictHas reports whether word is a headword in lang. Errors mean no
// dictionary is available for lang.
func dictHas(cfg *Config, word, lang string) (bool, error) {
	if !containsBackend(cfg.Backend.Word, "dictcc") {
		return false, errUnsupported
	}
	d, err := openDict(cfg)
	if err != nil {
		return false, err
	}
	col := d.column(lang)
	if col < 0 {
		return false, errUnsupported
	}
	lines, err := d.lookup(col, word, 1)
	return len(lines) > 0, err
}

func containsBackend(c chain, name string) bool {
	for _, n := range c {
		if n == name {
			return true
		}
	}
	return false
}

// dictKey normalises a term for lookup: annotations in {} [] <> () removed,
// lower case, English verbs without their "to ".
func dictKey(term, lang string) string {
	var b strings.Builder
	depth := 0
	for _, r := range term {
		switch r {
		case '{', '[', '<', '(':
			depth++
		case '}', ']', '>', ')':
			depth = max(depth-1, 0)
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	key := strings.ToLower(strings.Join(strings.Fields(b.String()), " "))
	if lang == "en" {
		key = strings.TrimPrefix(key, "to ")
	}
	return key
}

// splitTerm separates a dict.cc term into the text to copy and its
// annotations: "Gift {n} [chem.]" => "Gift", ["{n}", "[chem.]"].
// Parenthesised optional words like "(sich) verlieben" stay in the text.
func splitTerm(term string) (string, []string) {
	var text, ann strings.Builder
	var annotations []string
	depth := 0
	for _, r := range term {
		switch {
		case strings.ContainsRune("{[<", r):
			depth++
			ann.WriteRune(r)
		case strings.ContainsRune("}]>", r) && depth > 0:
			ann.WriteRune(r)
			if depth--; depth == 0 {
				annotations = append(annotations, ann.String())
				ann.Reset()
			}
		case depth > 0:
			ann.WriteRune(r)
		default:
			text.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(text.String()), " "), annotations
}
