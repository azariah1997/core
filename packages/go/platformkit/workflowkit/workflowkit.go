// Package workflowkit holds the one thing core-api (which starts/queries/
// signals workflows) and worker (which registers and executes them) must
// agree on: the Temporal task queue name. Workflow type names themselves
// stay free-form strings, like every other Type field in this repo - the
// task queue is the only actual coupling point, the same minimal-shared-
// contract role platformkit/searchidx.DefaultIndex plays for search.
package workflowkit

const TaskQueue = "platform-workflows"
