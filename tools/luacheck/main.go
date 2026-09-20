package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

func main() {
	path := flag.String("script-path", "bin/lua_scripts", "Lua script directory")
	probe := flag.Bool("probe", true, "run Lua string compatibility probes")
	flag.Parse()
	runtime := scripting.NewRuntime(scripting.Config{Enabled: true, ScriptPath: *path, Logger: slog.New(slog.NewTextHandler(os.Stdout, nil))})
	if err := runtime.Load(context.Background()); err != nil {
		fail(err)
	}
	failures := runtime.LoadFailures()
	for _, err := range failures {
		fmt.Println(err)
	}
	if len(failures) != 0 {
		os.Exit(1)
	}
	if *probe {
		if err := runtime.LoadString(`assert(string.match("#display 123", "#display") == "#display"); local words = {}; for word in ("one,two"):gmatch("([^,]+)") do words[#words + 1] = word end; assert(#words == 2 and words[1] == "one" and words[2] == "two"); assert(string.find("abc123", "%d+") == 4)`); err != nil {
			fail(err)
		}
	}
	fmt.Printf("lua scripts loaded=%d string_probes=%t\n", len(runtime.LoadedFiles()), *probe)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
