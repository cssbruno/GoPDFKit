// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPersistenceProcessHelper(t *testing.T) {
	mode := os.Getenv("GOPDFKIT_PERSISTENCE_HELPER")
	if mode == "" {
		return
	}
	root := os.Getenv("GOPDFKIT_PERSISTENCE_ROOT")
	workspace, err := OpenWorkspace(context.Background(), persistenceOptions(root))
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(20)
	}
	suffix := os.Getenv("GOPDFKIT_PERSISTENCE_SUFFIX")
	if _, err := workspace.CreateRevision("child-"+suffix+".paper", "document @child { page { text \""+suffix+"\" } }"); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(21)
	}
	switch mode {
	case "crash":
		point := os.Getenv("GOPDFKIT_PERSISTENCE_CRASH_POINT")
		persistenceFaultHook = func(got string) {
			if got == point {
				process, _ := os.FindProcess(os.Getpid())
				_ = process.Kill()
			}
		}
		if err := workspace.SaveSnapshot(context.Background()); err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(22)
		}
		os.Exit(23) // the requested fault point was not reached
	case "writer":
		ready, start := os.Getenv("GOPDFKIT_PERSISTENCE_READY"), os.Getenv("GOPDFKIT_PERSISTENCE_START")
		if err := os.WriteFile(ready, []byte("ready"), 0o600); err != nil {
			os.Exit(24)
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			if _, err := os.Stat(start); err == nil {
				break
			}
			if time.Now().After(deadline) {
				os.Exit(25)
			}
			time.Sleep(10 * time.Millisecond)
		}
		err := workspace.SaveSnapshot(context.Background())
		if err == nil {
			fmt.Print("committed")
			os.Exit(0)
		}
		if errors.Is(err, ErrPersistenceConflict) {
			fmt.Print("conflict")
			os.Exit(0)
		}
		fmt.Fprint(os.Stderr, err)
		os.Exit(26)
	default:
		os.Exit(27)
	}
}

func persistenceHelperCommand(root, mode, suffix string, extra ...string) *exec.Cmd {
	command := exec.Command(os.Args[0], "-test.run=^TestPersistenceProcessHelper$")
	command.Env = append(os.Environ(),
		"GOPDFKIT_PERSISTENCE_HELPER="+mode,
		"GOPDFKIT_PERSISTENCE_ROOT="+root,
		"GOPDFKIT_PERSISTENCE_SUFFIX="+suffix,
	)
	command.Env = append(command.Env, extra...)
	return command
}

func TestPersistenceCrashInjectionRecoversLastCommittedGeneration(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("advisory persistence locking is supported on Linux and macOS")
	}
	cases := []struct {
		point             string
		expectedRevisions int
	}{
		{point: "before_snapshot_write", expectedRevisions: 1},
		{point: "after_snapshot_write", expectedRevisions: 1},
		{point: "after_snapshot_fsync", expectedRevisions: 1},
		{point: "after_snapshot_replace", expectedRevisions: 1},
		{point: "after_snapshot_directory_fsync", expectedRevisions: 1},
		{point: "after_file_write", expectedRevisions: 1},
		{point: "after_file_fsync", expectedRevisions: 1},
		{point: "before_manifest_replace", expectedRevisions: 1},
		{point: "after_manifest_write", expectedRevisions: 1},
		{point: "after_manifest_fsync", expectedRevisions: 1},
		{point: "after_manifest_replace", expectedRevisions: 2},
		{point: "after_manifest_directory_fsync", expectedRevisions: 2},
	}
	for _, item := range cases {
		t.Run(item.point, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "state")
			workspace, err := OpenWorkspace(context.Background(), persistenceOptions(root))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := workspace.CreateRevision("base.paper", workspaceFixture); err != nil {
				t.Fatal(err)
			}
			if err := workspace.SaveSnapshot(context.Background()); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(root, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			command := persistenceHelperCommand(root, "crash", item.point, "GOPDFKIT_PERSISTENCE_CRASH_POINT="+item.point)
			err = command.Run()
			if err == nil {
				t.Fatal("crash helper unexpectedly completed")
			}
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				t.Fatalf("crash helper error = %v", err)
			}
			recovered, err := OpenWorkspace(context.Background(), persistenceOptions(root))
			if err != nil {
				t.Fatalf("recovery after %s: %v", item.point, err)
			}
			if len(recovered.revisions) != item.expectedRevisions {
				t.Fatalf("revisions after %s = %d, want %d", item.point, len(recovered.revisions), item.expectedRevisions)
			}
			after, _ := os.ReadFile(filepath.Join(root, "manifest.json"))
			if item.expectedRevisions == 1 && !bytes.Equal(before, after) {
				t.Fatalf("uncommitted crash changed manifest after %s", item.point)
			}
			if item.expectedRevisions == 2 && bytes.Equal(before, after) {
				t.Fatalf("committed manifest replacement was not recovered after %s", item.point)
			}
		})
	}
}

func TestPersistenceCoordinatesIndependentProcessWritersWithGenerationCAS(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("advisory persistence locking is supported on Linux and macOS")
	}
	base := t.TempDir()
	root := filepath.Join(base, "state")
	workspace, err := OpenWorkspace(context.Background(), persistenceOptions(root))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.CreateRevision("base.paper", workspaceFixture); err != nil {
		t.Fatal(err)
	}
	if err := workspace.SaveSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	start := filepath.Join(base, "start")
	type child struct {
		command *exec.Cmd
		output  bytes.Buffer
		ready   string
	}
	children := make([]child, 2)
	for index := range children {
		ready := filepath.Join(base, fmt.Sprintf("ready-%d", index))
		command := persistenceHelperCommand(root, "writer", fmt.Sprintf("%d", index), "GOPDFKIT_PERSISTENCE_READY="+ready, "GOPDFKIT_PERSISTENCE_START="+start)
		children[index] = child{command: command, ready: ready}
		command.Stdout, command.Stderr = &children[index].output, &children[index].output
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		ready := true
		for index := range children {
			if _, err := os.Stat(children[index].ready); err != nil {
				ready = false
			}
		}
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("writers did not reach the coordinated save boundary")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := os.WriteFile(start, []byte("start"), 0o600); err != nil {
		t.Fatal(err)
	}
	results := make([]string, len(children))
	for index := range children {
		if err := children[index].command.Wait(); err != nil {
			t.Fatalf("writer %d: %v: %s", index, err, children[index].output.String())
		}
		results[index] = strings.TrimSpace(children[index].output.String())
	}
	sortStrings(results)
	if results[0] != "committed" || results[1] != "conflict" {
		t.Fatalf("writer outcomes = %v", results)
	}
	recovered, err := OpenWorkspace(context.Background(), persistenceOptions(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered.revisions) != 2 || recovered.persistenceGeneration != 2 {
		t.Fatalf("committed state revisions/generation = %d/%d", len(recovered.revisions), recovered.persistenceGeneration)
	}
}

func sortStrings(values []string) {
	if len(values) == 2 && values[0] > values[1] {
		values[0], values[1] = values[1], values[0]
	}
}
