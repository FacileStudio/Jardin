package adapter

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
)

// mcpServerName is the key mycelium declares itself under.
const mcpServerName = "mycelium"

// mcpSubcommand is the argument that puts the binary on stdio as an MCP
// server. No adapter spells it out, so renaming the subcommand cannot leave
// one assistant pointed at a command that is gone.
const mcpSubcommand = "mcp"

// legacyServerName is what this tool shipped as before the 2026-08 rename.
//
// It is matched, never written. install left a jardin entry beside the new
// mycelium one pointing at a deleted binary, and `mycelium diff claude`
// reported no changes because the key it wanted was already present. A merge
// that replaces only by key reproduces that failure exactly.
const legacyServerName = "jardin"

// mcpBinary returns the command an MCP client should spawn.
//
// The running binary's own absolute path, not the bare name: a client spawns
// its servers directly rather than through a login shell, so ~/.local/bin is
// routinely off the PATH it inherits, and this machine has already had three
// older mycelium copies shadowing the installed one. Whichever binary you ran
// install with is the one the config points at, which is the only answer that
// is both reproducible and true.
func mcpBinary() string {
	exe, err := os.Executable()
	if err != nil {
		return mcpServerName
	}
	return exe
}

// mcpStdioServer is the declaration in the mcpServers shape Claude Code,
// Gemini CLI and Cursor all read: a command and its arguments over stdio.
//
// []any rather than []string so the value equals what unmarshalling the file
// we just wrote gives back, which is what makes a second install
// byte-identical rather than merely equivalent.
func mcpStdioServer() map[string]any {
	return map[string]any{
		"command": mcpBinary(),
		"args":    []any{mcpSubcommand},
	}
}

// mcpLocalServer is the same server in OpenCode's shape: a tagged transport
// and one argv array, with no separate args key. Its schema requires both
// fields and accepts no others as mandatory, so nothing else is set.
func mcpLocalServer() map[string]any {
	return map[string]any{
		"type":    "local",
		"command": []any{mcpBinary(), mcpSubcommand},
	}
}

// serverCommand returns the executable an existing entry spawns, across both
// shapes: a bare command string, and an argv array whose first element is the
// binary.
func serverCommand(entry map[string]any) string {
	switch command := entry["command"].(type) {
	case string:
		return command
	case []any:
		if len(command) > 0 {
			first, _ := command[0].(string)
			return first
		}
	}
	return ""
}

// isMyceliumServer reports whether an existing entry was written by this tool
// under any name it has ever shipped as.
//
// Matching the command as well as the key is the whole fix. An entry from an
// older name, or one somebody renamed by hand, is still mycelium's and still
// ours to remove — and invisible to a merge that only looks for its own key.
func isMyceliumServer(name string, entry map[string]any) bool {
	if name == mcpServerName || name == legacyServerName {
		return true
	}
	switch filepath.Base(serverCommand(entry)) {
	case mcpServerName, legacyServerName:
		return true
	}
	return false
}

// mergeMCPServers returns the full contents path should have with mycelium
// declared under key, whether that differs from what is already there, and
// whether there was anything safe to write at all.
//
// Merge rather than overwrite, because every one of these files also holds
// servers and settings mycelium did not put there. Replace rather than
// append, because the entry mycelium owns is mycelium's whatever it happens
// to be called today.
//
// Unparseable JSON is refused outright, matching InstallHooks: a file we
// cannot read is a file whose contents we would be guessing at, and this
// output is written over the original.
//
// The result is deterministic — encoding/json sorts map keys — so a second
// install re-reads what the first wrote and produces the same bytes. That is
// what makes `mycelium diff <agent>` clean on the second run rather than
// merely quiet about a duplicate it cannot see.
func mergeMCPServers(path, key string, server map[string]any) (string, bool, bool) {
	settings := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(data, &settings) != nil {
			return "", false, false
		}
	}

	servers, _ := settings[key].(map[string]any)
	if servers == nil {
		servers = make(map[string]any)
	}
	before := maps.Clone(servers)

	for name, entry := range servers {
		existing, _ := entry.(map[string]any)
		if isMyceliumServer(name, existing) {
			delete(servers, name)
		}
	}
	servers[mcpServerName] = server
	settings[key] = servers

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", false, false
	}
	return string(data) + "\n", !reflect.DeepEqual(before, servers), true
}

// declareMCPServer queues the merged declaration in out under the path it
// belongs at, and reports whether the agent ended up with one.
//
// Returning the answer rather than an error is what lets Generate set
// Input.MCPTools from the same call that emits the file: an agent whose
// config could not be parsed is not tool-capable this run, and must still be
// told to use the CLI.
func declareMCPServer(out *Output, path, key string, server map[string]any) bool {
	content, _, ok := mergeMCPServers(path, key, server)
	if !ok {
		return false
	}
	out.Files[path] = content
	return true
}

// canDeclareMCP reports whether path is a config this tool could merge into,
// without writing or queueing anything.
//
// For the one assistant whose MCP file is written outside Output.Files, so
// its rules still render for the audience it actually belongs to.
func canDeclareMCP(path, key string) bool {
	_, _, ok := mergeMCPServers(path, key, mcpStdioServer())
	return ok
}
