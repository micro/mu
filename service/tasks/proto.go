package tasks

import (
	"google.golang.org/protobuf/types/known/timestamppb"
	pb "mu/proto/service/tasks/v1"
	"time"
)

// Storage and execution records remain internal; the public shape is owned by proto.
func taskMessage(t *Task) *pb.Task {
	if t == nil {
		return nil
	}
	out := &pb.Task{Id: t.ID, Title: t.Title, Detail: t.Detail, Status: t.Status, Assignee: t.Assignee, Result: t.Result, Due: timestamp(t.Due), Created: timestamp(t.Created), Updated: timestamp(t.Updated), Owner: t.Owner, Thread: t.Thread, Agent: t.Agent, Archived: t.Archived, Steps: stepMessages(t.Steps)}
	if o := t.Occurrence; o != nil {
		out.Occurrence = &pb.Occurrence{Key: o.Key, Schedule: o.Schedule, Revision: o.Revision, Execution: o.Execution, Delivery: o.Delivery, State: o.State, Failure: o.Failure, MessageId: o.MessageID}
	}
	if d := t.Delivery; d != nil {
		out.Delivery = &pb.Delivery{Id: d.ID, Text: d.Text, From: d.From}
	}
	for _, a := range t.Attempts {
		out.Attempts = append(out.Attempts, &pb.Attempt{Id: a.ID, Started: timestamp(a.Started), Finished: timestamp(a.Finished), Version: a.Version, Status: a.Status, Steps: stepMessages(a.Steps), Report: a.Report, Error: a.Error})
	}
	return out
}
func stepMessages(steps []Step) []*pb.Step {
	var out []*pb.Step
	for _, s := range steps {
		out = append(out, &pb.Step{Id: s.ID, Started: timestamp(s.Started), Finished: timestamp(s.Finished), Status: s.Status, Args: s.Args, Output: s.Output, Error: s.Error, Tool: s.Tool, Detail: s.Detail, Ok: s.OK, Seconds: s.Seconds})
	}
	return out
}
func timestamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}
