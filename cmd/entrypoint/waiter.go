package main

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"os"
	"time"

	"github.com/tektoncd/pipeline/pkg/entrypoint"
)

// realWaiter actually waits for files, by polling.
type realWaiter struct {
	waitPollingInterval time.Duration
	breakpointOnFailure bool
}

var _ entrypoint.Waiter = (*realWaiter)(nil)

// setWaitPollingInterval sets the pollingInterval that will be used by the wait function
func (rw *realWaiter) setWaitPollingInterval(pollingInterval time.Duration) *realWaiter {
	rw.waitPollingInterval = pollingInterval
	return rw
}

// Wait watches a file and returns when either a) the file exists and, if
// the expectContent argument is true, the file has non-zero size or b) there
// is an error polling the file.
//
// If the passed-in file is an empty string then this function returns
// immediately.
//
// If a file of the same name with a ".err" extension exists then this Wait
// will end with a skipError.
func (rw *realWaiter) Wait(file string, expectContent bool, breakpointOnFailure bool) error {
	spew.Dump("Start of Wait")
	spew.Dump("File")
	spew.Dump(file)
	spew.Dump("Expecting Content")
	spew.Dump(expectContent)
	if file == "" {
		return nil
	}
	for ; ; time.Sleep(rw.waitPollingInterval) {
		info, err := os.Stat(file)
		spew.Dump("error while stating a file")
		spew.Dump(err)
		spew.Dump("size of a file")
		spew.Dump(info.Size())
		if err == nil {
			if !expectContent || info.Size() > 0 {
				return nil
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("waiting for %q: %w", file, err)
		}
		// When a .err file is read by this step, it means that a previous step has failed
		// We wouldn't want this step to stop executing because the previous step failed during debug
		// That is counterproductive to debugging
		// Hence we disable skipError here so that the other steps in the failed taskRun can continue
		// executing if breakpointOnFailure is enabled for the taskRun
		// TLDR: Do not return skipError when breakpointOnFailure is enabled as it breaks execution of the TaskRun
		if _, err := os.Stat(file + ".err"); err == nil {
			spew.Dump("error file exists")
			spew.Dump(file + ".err")
			if breakpointOnFailure {
				return nil
			}
			return skipError("error file present, bail and skip the step")
		}
		spew.Dump("end of the for loop")
	}
}

type skipError string

func (e skipError) Error() string {
	return string(e)
}
