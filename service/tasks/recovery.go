package tasks

import (
	"fmt"

	"mu/internal/userdb"
)

// recoverInterrupted runs during startup, before the task service accepts work.
// An old doing state cannot represent a live agent after a process restart.
// Do not replay it: a tool may have completed an external action before the
// process stopped. Leave the task blocked for the owner to inspect and retry.
func recoverInterrupted(owner string) error {
	if owner == "" {
		return nil
	}
	for {
		// Filter before limiting so tasks beyond the first page are recovered
		// too. Each successful update removes a record from the next batch.
		records, err := userdb.List(ns, owner, collection, "mine", map[string]interface{}{
			"status": StatusDoing, "assignee": Agent,
		}, "", "", userdb.MaxListLimit)
		if err != nil {
			return err
		}
		if len(records) == 0 {
			return nil
		}
		for _, rec := range records {
			task := toTask(rec.ID, rec.Owner, rec.Data)
			result := "This run was interrupted by a server restart. Review any actions already taken before running it again."
			if task.Result != "" {
				result += "\n\nPrevious result:\n" + task.Result
			}
			if _, err := Update(owner, task.ID, "", "", StatusBlocked, "", result); err != nil {
				return fmt.Errorf("recover task %s: %w", task.ID, err)
			}
		}
	}
}
