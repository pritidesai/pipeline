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

package dag

import (
	"errors"
	"fmt"
	"strings"

	"github.com/davecgh/go-spew/spew"

	"github.com/tektoncd/pipeline/pkg/list"
	"k8s.io/apimachinery/pkg/util/sets"
)

type Task interface {
	HashKey() string
	Deps() []string
}

type Tasks interface {
	Items() []Task
}

// Node represents a Task in a pipeline.
type Node struct {
	// Task represent the PipelineTask in Pipeline
	Task Task
	// Prev represent all the Previous task Nodes for the current Task
	Prev []*Node
	// Next represent all the Next task Nodes for the current Task
	Next []*Node
}

// Graph represents the Pipeline Graph
type Graph struct {
	//Nodes represent map of PipelineTask name to Node in Pipeline Graph
	Nodes map[string]*Node
}

// Returns an empty Pipeline Graph
func newGraph() *Graph {
	return &Graph{Nodes: map[string]*Node{}}
}

func (g *Graph) addPipelineTask(t Task) (*Node, error) {
	if _, ok := g.Nodes[t.HashKey()]; ok {
		return nil, errors.New("duplicate pipeline task")
	}
	newNode := &Node{
		Task: t,
	}
	g.Nodes[t.HashKey()] = newNode
	return newNode, nil
}

// Build returns a valid pipeline Graph. Returns error if the pipeline is invalid
func Build(tasks Tasks) (*Graph, error) {
	d := newGraph()

	deps := map[string][]string{}
	// Add all Tasks mentioned in the `PipelineSpec`
	for _, pt := range tasks.Items() {
		if _, err := d.addPipelineTask(pt); err != nil {
			return nil, fmt.Errorf("task %s is already present in Graph, can't add it again: %w", pt.HashKey(), err)
		}
		if len(pt.Deps()) != 0 {
			deps[pt.HashKey()] = pt.Deps()
		}
	}
	spew.Dump("deps")
	spew.Dump(deps)
	// Process all from and runAfter constraints to add task dependency
	for pt, taskDeps := range deps {
		spew.Printf("******* validating dependencies for pipelineTask %s *******\n", pt)
		spew.Dump("a list of parents")
		spew.Dump(taskDeps)
		for _, previousTask := range taskDeps {
			spew.Printf("adding a link between pipelineTask %s and parent %s\n", pt, previousTask)
			if err := addLink(pt, previousTask, d.Nodes); err != nil {
				return nil, fmt.Errorf("couldn't add link between %s and %s: %w", pt, previousTask, err)
			}
		}
	}
	return d, nil
}

// GetSchedulable returns a map of PipelineTask that can be scheduled (keyed
// by the name of the PipelineTask) given a list of successfully finished doneTasks.
// It returns tasks which have all dependencies marked as done, and thus can be scheduled. If the
// specified doneTasks are invalid (i.e. if it is indicated that a Task is
// done, but the previous Tasks are not done), an error is returned.
func GetSchedulable(g *Graph, doneTasks ...string) (sets.String, error) {
	roots := getRoots(g)
	tm := sets.NewString(doneTasks...)
	d := sets.NewString()

	visited := sets.NewString()
	for _, root := range roots {
		schedulable := findSchedulable(root, visited, tm)
		for _, task := range schedulable {
			d.Insert(task.HashKey())
		}
	}

	var visitedNames []string
	for v := range visited {
		visitedNames = append(visitedNames, v)
	}

	notVisited := list.DiffLeft(doneTasks, visitedNames)
	if len(notVisited) > 0 {
		return nil, fmt.Errorf("invalid list of done tasks; some tasks were indicated completed without ancestors being done: %v", notVisited)
	}

	return d, nil
}

func linkPipelineTasks(prev *Node, next *Node) error {
	// Check for self cycle
	if prev.Task.HashKey() == next.Task.HashKey() {
		return fmt.Errorf("cycle detected; task %q depends on itself", next.Task.HashKey())
	}
	// Check if we are adding cycles.
	visited := map[string]bool{prev.Task.HashKey(): true, next.Task.HashKey(): true}
	path := []string{next.Task.HashKey(), prev.Task.HashKey()}
	spew.Dump("calling visit with")
	spew.Printf("next %s\n", next.Task.HashKey())
	spew.Printf("prev %s\n", prev.Task.HashKey())
	spew.Dump("printing visited")
	spew.Dump(visited)
	spew.Dump("printing prev.Prev")
	if len(prev.Prev) == 0 {
		spew.Dump("prev.Prev is empty")
	}
	for _, p := range prev.Prev {
		spew.Dump(p.Task.HashKey())
	}
	if err := visit(next.Task.HashKey(), prev.Prev, path, visited); err != nil {
		//if err := visit(next.Task.HashKey(), prev.Prev, path, visited); err != nil {
		return fmt.Errorf("cycle detected: %w", err)
	}
	next.Prev = append(next.Prev, prev)
	prev.Next = append(prev.Next, next)
	return nil
}

//func visit(currentName string, nodes []*Node, path []string, visited map[string]bool) error {

func visit(currentName string, nodes []*Node, path []string, visited map[string]bool) error {
	var sb strings.Builder
	for _, n := range nodes {
		path = append(path, n.Task.HashKey())
		if _, ok := visited[n.Task.HashKey()]; ok {
			spew.Printf("visited has the %s task already", n.Task.HashKey())
			return errors.New(getVisitedPath(path))
		}
		spew.Dump("calling visit with")
		spew.Dump("n.Prev")
		if len(n.Prev) == 0 {
			spew.Dump("n.Prev is empty")
		}
		for _, p := range n.Prev {
			spew.Dump(p.Task.HashKey())
		}
		sb.WriteString(currentName)
		sb.WriteByte('.')
		sb.WriteString(n.Task.HashKey())
		spew.Printf("currentName %s\n", currentName)
		spew.Printf("Task hash key %s\n", n.Task.HashKey())
		spew.Printf("sb %s\n", sb.String())
		visited[sb.String()] = true

		spew.Dump("path")
		spew.Dump(getVisitedPath(path))
		spew.Dump("visited")
		spew.Dump(visited)
		if err := visit(n.Task.HashKey(), n.Prev, path, visited); err != nil {
			//if err := visit(n.Task.HashKey(), n.Prev, path, visited); err != nil {
			return err
		}
	}
	return nil
}

//var sb strings.Builder
//sb.WriteString(currentName)
//sb.WriteByte('.')
//sb.WriteString(n.Task.HashKey())
//spew.Dump("currentName+TaskName")
//spew.Dump(sb.String())
//visited[sb.String()] = true

func getVisitedPath(path []string) string {
	// Reverse the path since we traversed the Graph using prev pointers.
	for i := len(path)/2 - 1; i >= 0; i-- {
		opp := len(path) - 1 - i
		path[i], path[opp] = path[opp], path[i]
	}
	return strings.Join(path, " -> ")
}

func addLink(pt string, previousTask string, nodes map[string]*Node) error {
	prev, ok := nodes[previousTask]
	if !ok {
		return fmt.Errorf("task %s depends on %s but %s wasn't present in Pipeline", pt, previousTask, previousTask)
	}
	next := nodes[pt]
	if err := linkPipelineTasks(prev, next); err != nil {
		return fmt.Errorf("couldn't create link from %s to %s: %w", prev.Task.HashKey(), next.Task.HashKey(), err)
	}
	return nil
}

func getRoots(g *Graph) []*Node {
	n := []*Node{}
	for _, node := range g.Nodes {
		if len(node.Prev) == 0 {
			n = append(n, node)
		}
	}
	return n
}

func findSchedulable(n *Node, visited sets.String, doneTasks sets.String) []Task {
	if visited.Has(n.Task.HashKey()) {
		return []Task{}
	}
	visited.Insert(n.Task.HashKey())
	if doneTasks.Has(n.Task.HashKey()) {
		schedulable := []Task{}
		// This one is done! Take note of it and look at the next candidate
		for _, next := range n.Next {
			if _, ok := visited[next.Task.HashKey()]; !ok {
				schedulable = append(schedulable, findSchedulable(next, visited, doneTasks)...)
			}
		}
		return schedulable
	}
	// This one isn't done! Return it if it's schedulable
	if isSchedulable(doneTasks, n.Prev) {
		// FIXME(vdemeester)
		return []Task{n.Task}
	}
	// This one isn't done, but it also isn't ready to schedule
	return []Task{}
}

func isSchedulable(doneTasks sets.String, prevs []*Node) bool {
	if len(prevs) == 0 {
		return true
	}
	collected := []string{}
	for _, n := range prevs {
		if doneTasks.Has(n.Task.HashKey()) {
			collected = append(collected, n.Task.HashKey())
		}
	}
	return len(collected) == len(prevs)
}
