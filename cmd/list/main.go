// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// Command list prints Markdown links for generated reference PDFs.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func matchTail(str, tailStr string) (match bool, headStr string) {
	sln := len(str)
	ln := len(tailStr)
	if sln > ln {
		match = str[sln-ln:] == tailStr
		if match {
			headStr = str[:sln-ln]
		}
	}
	return
}

func matchHead(str, headStr string) (match bool, tailStr string) {
	ln := len(headStr)
	if len(str) > ln {
		match = str[:ln] == headStr
		if match {
			tailStr = str[ln:]
		}
	}
	return
}

func main() {
	var err error
	var ok bool
	var showStr, name string
	err = filepath.Walk("assets/generated/pdf/reference", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info == nil {
			return nil
		}
		if info.Mode().IsRegular() {
			name = filepath.Base(path)
			ok, name = matchTail(name, ".pdf")
			if ok {
				name = strings.ReplaceAll(name, "_", " ")
				ok, showStr = matchHead(name, "Document ")
				if ok {
					fmt.Printf("[%s](%s)\n", showStr, path)
				} else {
					for _, prefix := range []string{"barcode ", "thumb "} {
						ok, showStr = matchHead(name, prefix)
						if ok {
							fmt.Printf("[%s](%s)\n", showStr, path)
							break
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		fmt.Println(err)
	}
}
