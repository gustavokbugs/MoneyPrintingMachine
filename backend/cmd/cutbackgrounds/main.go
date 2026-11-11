// Command cutbackgrounds splits long background videos in
// .local/media/backgrounds/sources into one-minute vertical clips in
// .local/media/backgrounds, where gameplay stories pick them up. Run it
// from the backend folder after adding sources; finished clips are kept.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"contentfactory/internal/production"
)

func main() {
	count := flag.Int("count", 10, "clips per source video")
	seconds := flag.Float64("seconds", 60, "length of each clip in seconds")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	localDir := filepath.Join("..", ".local")
	tools := production.ToolsFromRoot(localDir)
	made, err := production.CutBackgrounds(ctx, tools, production.ExecRunner{}, filepath.Join(localDir, "media"), *count, *seconds, 1080, 1920)
	for _, m := range made {
		fmt.Println("made", m)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%d new clips\n", len(made))
}
