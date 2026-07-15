// Package main implements cmd/translation-progress.
//
// translation-progress scans every .go file in the repository and extracts
// the translation metadata header (Translation of / Completeness /
// Simplifications). It prints a per-file report and a summary statistic.
//
// Header format expected at the top of each translated file:
//
//	// Translation of: Source/WebCore/rendering/RenderBlockFlow.h
//	//                  Source/WebCore/rendering/RenderBlockFlow.cpp
//	// Completeness: 95%
//	// Simplifications:
//	//   - line-height shrink factor uses naive 1.2
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type fileRecord struct {
	Path          string
	TranslationOf []string
	Completeness  int // 0-100, -1 if not specified
	Simplifications []string
}

func main() {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "getwd:", err)
		os.Exit(1)
	}

	var records []fileRecord
	err = filepath.WalkDir(wd, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".trae" || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rec := parseFile(path)
		records = append(records, rec)
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "walk:", err)
		os.Exit(1)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Path < records[j].Path
	})

	total := 0
	translated := 0
	sumCompleteness := 0
	countCompleteness := 0

	fmt.Println("# wb-ui Translation Progress")
	fmt.Println()
	for _, r := range records {
		rel, _ := filepath.Rel(wd, r.Path)
		total++
		if len(r.TranslationOf) > 0 {
			translated++
			fmt.Printf("- %s\n", rel)
			for _, t := range r.TranslationOf {
				fmt.Printf("    Translation of: %s\n", t)
			}
			if r.Completeness >= 0 {
				fmt.Printf("    Completeness: %d%%\n", r.Completeness)
				sumCompleteness += r.Completeness
				countCompleteness++
			} else {
				fmt.Printf("    Completeness: (unspecified)\n")
			}
			if len(r.Simplifications) > 0 {
				fmt.Println("    Simplifications:")
				for _, s := range r.Simplifications {
					fmt.Printf("      - %s\n", s)
				}
			}
		}
	}

	fmt.Println()
	fmt.Println("## Summary")
	fmt.Printf("- Total .go files: %d\n", total)
	fmt.Printf("- Files with Translation of header: %d (%.1f%%)\n", translated, percent(translated, total))
	if countCompleteness > 0 {
		avg := float64(sumCompleteness) / float64(countCompleteness)
		fmt.Printf("- Average completeness (of translated files with header): %.1f%%\n", avg)
	} else {
		fmt.Println("- Average completeness: n/a")
	}
}

func percent(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d) * 100
}

func parseFile(path string) fileRecord {
	f, err := os.Open(path)
	if err != nil {
		return fileRecord{Path: path, Completeness: -1}
	}
	defer f.Close()

	rec := fileRecord{Path: path, Completeness: -1}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	inSimplifications := false
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		if lineNo > 60 {
			// Header should be at the top of the file.
			break
		}
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Stop at the first non-comment, non-blank line (e.g. package clause).
		if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
			break
		}

		switch {
		case strings.HasPrefix(trimmed, "// Translation of:"):
			rest := strings.TrimPrefix(trimmed, "// Translation of:")
			rest = strings.TrimSpace(rest)
			if rest != "" {
				rec.TranslationOf = append(rec.TranslationOf, rest)
			}
			inSimplifications = false
		case strings.HasPrefix(trimmed, "// Completeness:"):
			rest := strings.TrimPrefix(trimmed, "// Completeness:")
			rest = strings.TrimSpace(rest)
			rest = strings.TrimSuffix(rest, "%")
			if v, err := strconv.Atoi(rest); err == nil {
				rec.Completeness = v
			}
			inSimplifications = false
		case strings.HasPrefix(trimmed, "// Simplifications:"):
			inSimplifications = true
		case inSimplifications && strings.HasPrefix(trimmed, "//"):
			rest := strings.TrimPrefix(trimmed, "//")
			rest = strings.TrimSpace(rest)
			if strings.HasPrefix(rest, "- ") {
				rest = strings.TrimPrefix(rest, "- ")
			}
			if rest != "" {
				rec.Simplifications = append(rec.Simplifications, rest)
			}
		default:
			inSimplifications = false
		}
	}
	return rec
}
