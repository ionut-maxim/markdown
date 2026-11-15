// Command mdview shows a markdown file in the terminal, for checking the
// renderer's output by eye:
//
//	go run github.com/ionut-maxim/markdown/cmd/mdview file.md
//
// It's a pager built on teamd. Scroll with the arrow keys, j and k, space
// and b, the mouse wheel or a drag. t forces the text fallback, d switches
// dark and light, [ and ] change the width, and q quits. The bottom line
// shows what it found out about the terminal.
//
// Images come from paths next to the file. http and https images are
// fetched only with -fetch. With -print, the file is printed straight to
// the terminal instead, where kitty's text sizing can draw the headings.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

func main() {
	fetchWeb := flag.Bool("fetch", false, "fetch http and https images")
	printMode := flag.Bool("print", false, "print straight to the terminal (shows kitty text sizing) instead of paging")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mdview [-fetch] [-print] file.md")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	path := flag.Arg(0)
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fetch := fetcher(filepath.Dir(path), *fetchWeb)
	if *printMode {
		err = printDoc(string(src), fetch)
	} else {
		_, err = tea.NewProgram(newPager(string(src), fetch)).Run()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// fetcher gets images: files relative to dir, and web images when web is
// set.
func fetcher(dir string, web bool) func(context.Context, string) ([]byte, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	return func(ctx context.Context, url string) ([]byte, error) {
		if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
			if !web {
				return nil, errors.New("not fetched: run with -fetch")
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return nil, err
			}
			resp, err := client.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return nil, errors.New(resp.Status)
			}
			return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		}
		if !filepath.IsAbs(url) {
			url = filepath.Join(dir, url)
		}
		return os.ReadFile(url)
	}
}
