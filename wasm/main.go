//go:build js && wasm

// Command wasm runs zanadir in a web page. It sets zanadirScan(repo, format)
// on the page, which returns a promise of the report, and zanadir, which
// holds the version and the category names.
package main

import (
	"bytes"
	"cmp"
	"context"
	"syscall/js"

	"github.com/MustacheCase/zanadir/app"
	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/handler"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/remote"
)

func main() {
	categories := make([]any, 0, len(models.CategoryTitles))
	for _, name := range models.CategoryNames() {
		categories = append(categories, name)
	}
	js.Global().Set("zanadir", js.ValueOf(map[string]any{"version": app.Version(), "categories": categories}))
	js.Global().Set("zanadirScan", js.FuncOf(scan))
	select {}
}

// scan must not block: the page's event loop is what answers the fetches
// it waits on.
func scan(_ js.Value, args []js.Value) any {
	var target, format string
	if len(args) > 0 {
		target = args[0].String()
	}
	if len(args) > 1 {
		format = args[1].String()
	}
	executor := js.FuncOf(func(_ js.Value, promise []js.Value) any {
		resolve, reject := promise[0], promise[1]
		go func() {
			report, err := run(target, cmp.Or(format, config.OutputJSON))
			if err != nil {
				reject.Invoke(js.Global().Get("Error").New(err.Error()))
				return
			}
			resolve.Invoke(report)
		}()
		return nil
	})
	defer executor.Release()
	return js.Global().Get("Promise").New(executor)
}

func run(target, format string) (string, error) {
	repo, err := remote.Parse(target)
	if err != nil {
		return "", err
	}
	dir, cleanup, err := remote.FetchContext(context.Background(), repo, "")
	if err != nil {
		return "", err
	}
	defer cleanup()
	h, err := handler.Setup()
	if err != nil {
		return "", err
	}
	return scanDir(h, dir, format)
}

func scanDir(h *handler.Handler, dir, format string) (string, error) {
	var out bytes.Buffer
	err := h.Execute(&config.Config{Dir: dir, Output: format, Out: &out})
	return out.String(), err
}
