// Copyright (c) 2013 The Gocov Authors.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rabadin/gocov/gocov/internal/testflag"
)

// createMissingTestFiles creates test files for all go files without tests.
// This is to work around https://github.com/axw/gocov/issues/81 and
// https://github.com/golang/go/issues/24570.
func createMissingTestFiles(pkgs []string) ([]string, error) {
	var buf bytes.Buffer

	cmd := exec.Command("go", append([]string{"list", "-f", "{{.Dir}} {{.Name}} {{.GoFiles}}"}, pkgs...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	matcher := regexp.MustCompile(`(\S+) (.*) \[(.*)\]`)
	var createdFiles []string
	lines := strings.Split(buf.String(), "\n")
	for _, line := range lines {
		matches := matcher.FindStringSubmatch(line)
		if len(matches) < 4 {
			continue
		}
		packagePath := matches[1]
		packageName := matches[2]
		goFiles := matches[3]

		// Create empty files.
		for _, goFile := range strings.Split(goFiles, " ") {
			testFile := strings.TrimSuffix(goFile, ".go") + "_test.go"
			testFilePath := path.Join(packagePath, testFile)
			newFile, err := os.OpenFile(testFilePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
			if err != nil && !os.IsExist(err) {
				return nil, err
			} else if os.IsExist(err) {
				continue
			}

			if _, err := newFile.Write([]byte("package " + packageName + "\n")); err != nil {
				return nil, err
			}
			if err := newFile.Close(); err != nil {
				return nil, err
			}
			createdFiles = append(createdFiles, testFilePath)
		}
	}
	return createdFiles, nil
}

func deleteCreateTestFiles(files []string) error {
	for _, toDeleteFile := range files {
		var err = os.Remove(toDeleteFile)
		if err != nil {
			return err
		}
	}
	return nil
}

func runTests(args []string) error {
	pkgs, testFlags := testflag.Split(args)
	newFiles, err := createMissingTestFiles(pkgs)
	defer func() {
		deleteCreateTestFiles(newFiles)
	}()
	if err != nil {
		return err
	}

	tmpDir, err := ioutil.TempDir("", "gocov")
	if err != nil {
		return err
	}
	defer func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			log.Printf("failed to clean up temp directory %q", tmpDir)
		}
	}()

	coverFile := filepath.Join(tmpDir, "cover.cov")
	cmdArgs := append([]string{"test", "-coverprofile", coverFile}, testFlags...)
	for _, pkg := range pkgs {
		cmdArgs = append(cmdArgs, pkg)
	}
	cmd := exec.Command("go", cmdArgs...)
	cmd.Stdin = nil
	// Write all test command output to stderr so as not to interfere with
	// the JSON coverage output.
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}

	files, err := filepath.Glob(filepath.Join(tmpDir, "cover.cov"))
	if err != nil {
		return err
	}

	// Merge the profiles.
	return convertProfiles(files...)
}
