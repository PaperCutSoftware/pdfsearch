// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package cmd_utils

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar"
	"github.com/unidoc/unipdf/v3/common"
)

// PatternsToPaths returns a list of files matching the patterns in `patternList`.
// The returned list is sorted alphabetically .
func PatternsToPaths(patternList []string) ([]string, error) {
	var pathList []string
	common.Log.Debug("patternList=%d", len(patternList))
	for i, pattern := range patternList {
		pattern = ExpandUser(pattern)
		files, err := doublestar.Glob(pattern)
		if err != nil {
			common.Log.Error("PatternsToPaths: Glob failed. pattern=%#q err=%v", pattern, err)
			return pathList, err
		}
		common.Log.Debug("patternList[%d]=%q %d matches", i, pattern, len(files))
		for _, filename := range files {
			ok, err := RegularFile(filename)
			if err != nil {
				common.Log.Error("PatternsToPaths: RegularFile failed. pattern=%#q err=%v", pattern, err)
				return pathList, err
			}
			if !ok {
				continue
			}
			pathList = append(pathList, filename)
		}
	}
	pathList = StringUniques(pathList)
	sort.Strings(pathList)
	return pathList, nil
}

// Shuffle returns a deterministically shuffled copy of `pathList`. The shuffled order should be
// uncorrelated with the alphabetically sorted `pathList`.
// This intended lack of correlation relies on the FNV-1a hash of a string being uncorrelated with
// the string.
func Shuffle(pathList []string) []string {
	pathHash := map[string]uint64{}
	for _, path := range pathList {
		pathHash[path] = hash(path)
	}
	if len(pathHash) < len(pathList) {
		common.Log.Info("Shuffle: %d collisions in %d file names", len(pathList)-len(pathHash),
			len(pathList))
	}
	sort.Slice(pathList, func(i, j int) bool {
		pi, pj := pathList[i], pathList[j]
		hi, hj := pathHash[pi], pathHash[pj]
		if hi != hj {
			return hi < hj
		}
		// In the remote chance of a hash collision, we sort by reversed string order.
		return reverse(pi) < reverse(pj)
	})
	return pathList
}

// hash return the 64 bit FNV-1a hash of `s`.
// See https://en.wikipedia.org/wiki/Fowler%E2%80%93Noll%E2%80%93Vo_hash_function#FNV-1a_hash
func hash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// reverse returns `s` with the characters reversed.
func reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// FileFinder is a group of file paths.
type FileFinder struct {
	// namePaths is a map {base name: all file paths with this base name}
	namePaths map[string][]string
}

// NewFileFinder returns a FileFinder of all file paths in `pathList`.
func NewFileFinder(pathList []string) FileFinder {
	var ff FileFinder
	ff.namePaths = map[string][]string{}
	for _, fullpath := range pathList {
		name := filepath.Base(fullpath)
		ff.namePaths[name] = append(ff.namePaths[name], fullpath)
	}
	return ff
}

// NewFileFinderFromCorpus returns a FileFinder for all files in our main corpus directory.
func NewFileFinderFromCorpus() (FileFinder, error) {
	patternList := []string{"~/testdata/**/*.pdf"}
	pathList, err := PatternsToPaths(patternList)
	if err != nil {
		return FileFinder{}, err
	}
	return NewFileFinder(pathList), nil
}

// Find finds the file path in `ff` that best matches `fullpath`.
func (ff *FileFinder) Find(fullpath string) string {
	name := filepath.Base(fullpath)
	pathList, ok := ff.namePaths[name]
	if !ok {
		fmt.Printf("$$1 No match.   %40q : %q\n", name, fullpath)
		return ""
	} else if len(pathList) > 1 {
		best := longestMatchingSuffix(fullpath, pathList)
		fmt.Printf("$$2 Duplicates. %40q:\n -- %100q\n -- %100q\n", name, fullpath, best)
		for i, p := range pathList {
			fmt.Printf("%6d: %q\n", i, p)
		}
		return best
	}
	return pathList[0]
}

// longestMatchingSuffix returns the string in `stringList` that has the longest matching suffix
// with `str`.
func longestMatchingSuffix(str string, stringList []string) string {
	sort.SliceStable(stringList, func(i, j int) bool {
		si, sj := stringList[i], stringList[j]
		ni, nj := len(si), len(sj)
		if ni != nj {
			return ni < nj
		}
		return si < sj
	})
	best_s, best_n := "", 0
	for _, s := range stringList {
		n := commonSuffix(s, str)
		if n > best_n {
			best_s, best_n = s, n
		}
	}
	return best_s
}

// commonSuffix returns the number of characters in the common suffix of `s1` and `s2`.
func commonSuffix(s1, s2 string) int {
	n1, n2 := len(s1), len(s2)
	n := n1
	if n2 < n {
		n = n2
	}
	i := 0
	for ; i < n; i++ {
		if s1[n1-1-i] != s2[n2-1-i] {
			break
		}
	}
	return i
}

// homeDir is the current user's home directory.
var homeDir = getHomeDir()

// getHomeDir returns the current user's home directory.
func getHomeDir() string {
	usr, _ := user.Current()
	return usr.HomeDir
}

// ExpandUser returns `filename` with "~"" replaced with user's home directory.
func ExpandUser(filename string) string {
	return strings.Replace(filename, "~", homeDir, -1)
}

// SortFileSize returns the paths of the files in `pathList` sorted by ascending size.
// If `minSize` >= 0 then only files of this size or larger are returned.
// If `maxSize` >= 0 then only files of this size or smaller are returned.
func SortFileSize(pathList []string, minSize, maxSize int64) ([]string, error) {

	sort.Slice(pathList, func(i, j int) bool {
		pi, pj := pathList[i], pathList[j]
		si, _ := FileSize(pi)
		sj, _ := FileSize(pj)
		if si != sj {
			return si > sj
		}
		return pi < pj
	})

	i0 := 0
	i1 := len(pathList)
	if minSize >= 0 {
		i0 = sort.Search(len(pathList), func(i int) bool {
			size, _ := FileSize(pathList[i])
			return size >= minSize
		})
	}
	if maxSize >= 0 {
		i1 = sort.Search(len(pathList), func(i int) bool {
			size, _ := FileSize(pathList[i])
			return size >= maxSize
		})
	}
	pathList = pathList[i0:i1]
	return pathList, nil
}

type fileInfo struct {
	filename string
	os.FileInfo
}

// RegularFile returns true if file `filename` is a regular file.
func RegularFile(filename string) (bool, error) {
	fi, err := os.Stat(filename)
	if err != nil {
		return false, err
	}
	return fi.Mode().IsRegular(), nil
}

// FileSize returns the size of file `filename` in bytes.
func FileSize(filename string) (int64, error) {
	fi, err := os.Stat(filename)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// FileHashSize is the maximum number of hexidecimal digits returned for file hashes.
var FileHashSize = 10

// FileHash returns a hex encoded string of the SHA-256 digest of the contents of file `filename`.
func FileHash(filename string) (string, error) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return makeHash(b), nil
}

// makeHash returns a hex encoded string of the SHA-256 digest of `b.
func makeHash(b []byte) string {
	hasher := sha256.New()
	hasher.Write(b)
	digest := hex.EncodeToString(hasher.Sum(nil))
	if FileHashSize > 0 && FileHashSize < len(digest) {
		digest = digest[:FileHashSize]
	}
	return digest
}

// StringUniques returns the unique strings in `arr`.
func StringUniques(arr []string) []string {
	set := map[string]bool{}
	var u []string
	for _, s := range arr {
		if _, ok := set[s]; !ok {
			u = append(u, s)
		}
		set[s] = true
	}
	return u
}
