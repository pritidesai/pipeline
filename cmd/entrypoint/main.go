/*
Copyright 2019 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/tektoncd/pipeline/cmd/entrypoint/subcommands"
	"github.com/tektoncd/pipeline/pkg/credentials"
	"github.com/tektoncd/pipeline/pkg/credentials/dockercreds"
	"github.com/tektoncd/pipeline/pkg/credentials/gitcreds"
	"github.com/tektoncd/pipeline/pkg/entrypoint"
	"github.com/tektoncd/pipeline/pkg/termination"
)

var (
	ep              = flag.String("entrypoint", "", "Original specified entrypoint to execute")
	waitFiles       = flag.String("wait_file", "", "Comma-separated list of paths to wait for")
	waitFileContent = flag.Bool("wait_file_content", false, "If specified, expect wait_file to have content")
	postFile        = flag.String("post_file", "", "If specified, file to write upon completion")
	terminationPath = flag.String("termination_path", "/tekton/termination", "If specified, file to write upon termination")
	results         = flag.String("results", "", "If specified, list of file names that might contain task results")
	timeout         = flag.Duration("timeout", time.Duration(0), "If specified, sets timeout for step")
	exitCode        = flag.Int("exit_code", -1, "if specified, sets the exit code for the step")
	variables       = flag.String("variables", "", "if specified, list of files names that might contain the step data")
	exitCodeFile    = flag.String("exit_code_file", "", "If specified, file to write exit code of the container")
)

const defaultWaitPollingInterval = time.Second

func main() {
	// Add credential flags originally introduced with our legacy credentials helper
	// image (creds-init).
	gitcreds.AddFlags(flag.CommandLine)
	dockercreds.AddFlags(flag.CommandLine)

	flag.Parse()

	if err := subcommands.Process(flag.Args()); err != nil {
		log.Println(err.Error())
		switch err.(type) {
		case subcommands.SubcommandSuccessful:
			return
		default:
			os.Exit(1)
		}
	}

	// Copy credentials we're expecting from the legacy credentials helper (creds-init)
	// from secret volume mounts to /tekton/creds. This is done to support the expansion
	// of a variable, $(credentials.path), that resolves to a single place with all the
	// stored credentials.
	builders := []credentials.Builder{dockercreds.NewBuilder(), gitcreds.NewBuilder()}
	for _, c := range builders {
		if err := c.Write("/tekton/creds"); err != nil {
			log.Printf("Error initializing credentials: %s", err)
		}
	}

	e := entrypoint.Entrypointer{
		Entrypoint:      *ep,
		WaitFiles:       strings.Split(*waitFiles, ","),
		WaitFileContent: *waitFileContent,
		PostFile:        *postFile,
		TerminationPath: *terminationPath,
		Args:            flag.Args(),
		Waiter:          &realWaiter{waitPollingInterval: defaultWaitPollingInterval},
		Runner:          &realRunner{},
		PostWriter:      &realPostWriter{},
		Results:         strings.Split(*results, ","),
		Timeout:         timeout,
		ExitCode:        *exitCode,
		Variables:       strings.Split(*variables, ","),
		ExitCodeFile:    *exitCodeFile,
	}

	// Copy any creds injected by the controller into the $HOME directory of the current
	// user so that they're discoverable by git / ssh.
	if err := credentials.CopyCredsToHome(credentials.CredsInitCredentials); err != nil {
		log.Printf("non-fatal error copying credentials: %q", err)
	}

	if err := e.Go(); err != nil {
		switch t := err.(type) {
		case skipError:
			log.Print("Skipping step because a previous step failed")
			os.Exit(1)
		case termination.MessageLengthError:
			log.Print(err.Error())
			os.Exit(1)
		case *exec.ExitError:
			// Copied from https://stackoverflow.com/questions/10385551/get-exit-code-go
			// This works on both Unix and Windows. Although
			// package syscall is generally platform dependent,
			// WaitStatus is defined for both Unix and Windows and
			// in both cases has an ExitStatus() method with the
			// same signature.
			if status, ok := t.Sys().(syscall.WaitStatus); ok {
				// log the original exit code in the container logs
				// if exit code is specified by the user i.e. != -1
				log.Printf("Entrypoint exit code: %d", e.ExitCode)
				if e.ExitCode != -1 {
					log.Printf("original exit code executing command (ExitError): %v", status.ExitStatus())
				}
				// exit only if the exit code specified by the user is not equal to 0
				if e.ExitCode != 0 {
					os.Exit(status.ExitStatus())
				}
			}
			// log and exit only if the exit code specified by the user is not equal to 0
			if e.ExitCode != 0 {
				log.Fatalf("Error executing command (ExitError): %v", err)
			}
		default:
			log.Fatalf("Error executing command: %v", err)
		}
	}
}
