package fcpcore

import "fmt"

// checkpointEntry is a sentinel stored in the event log to mark a checkpoint.
type checkpointEntry struct {
	name string
}

// EventLog is a generic cursor-based event log with undo/redo and named checkpoints.
//
// Events are appended at the cursor position. The cursor always points
// one past the last applied event. Undo moves the cursor back; redo
// moves it forward. Appending a new event truncates the redo tail.
//
// Checkpoint sentinels are stored in the log but skipped during
// undo/redo traversal.
type EventLog struct {
	events      []any
	cursor      int
	checkpoints map[string]int
}

// NewEventLog creates a new empty EventLog.
func NewEventLog() *EventLog {
	return &EventLog{
		checkpoints: make(map[string]int),
	}
}

// Append adds an event, truncating any redo history beyond the cursor.
func (l *EventLog) Append(event any) {
	if l.cursor < len(l.events) {
		l.events = l.events[:l.cursor]
		// Remove checkpoints pointing beyond new length
		for name, idx := range l.checkpoints {
			if idx > l.cursor {
				delete(l.checkpoints, name)
			}
		}
	}
	l.events = append(l.events, event)
	l.cursor = len(l.events)
}

// Checkpoint creates a named checkpoint at the current cursor position.
func (l *EventLog) Checkpoint(name string) {
	l.checkpoints[name] = l.cursor
	if l.cursor < len(l.events) {
		l.events = l.events[:l.cursor]
		// Remove checkpoints pointing beyond new length
		for n, idx := range l.checkpoints {
			if idx > l.cursor {
				delete(l.checkpoints, n)
			}
		}
	}
	l.events = append(l.events, checkpointEntry{name: name})
	l.cursor = len(l.events)
}

// isCheckpoint returns true if the entry is a checkpoint sentinel.
func isCheckpoint(entry any) bool {
	_, ok := entry.(checkpointEntry)
	return ok
}

// Undo undoes up to count non-checkpoint events. Returns events in reverse
// order (most recent first) for the caller to reverse-apply.
func (l *EventLog) Undo(count int) []any {
	var result []any
	pos := l.cursor - 1
	undone := 0

	for pos >= 0 && undone < count {
		entry := l.events[pos]
		if !isCheckpoint(entry) {
			result = append(result, entry)
			undone++
		}
		pos--
	}

	l.cursor = pos + 1
	return result
}

// UndoTo undoes to a named checkpoint. Returns events in reverse order.
// Returns nil and an error if the checkpoint doesn't exist or is at/beyond cursor.
func (l *EventLog) UndoTo(name string) ([]any, error) {
	target, ok := l.checkpoints[name]
	if !ok || target >= l.cursor {
		return nil, fmt.Errorf("cannot undo to %q", name)
	}

	var result []any
	for i := l.cursor - 1; i >= target; i-- {
		entry := l.events[i]
		if !isCheckpoint(entry) {
			result = append(result, entry)
		}
	}
	l.cursor = target
	return result, nil
}

// Redo redoes up to count non-checkpoint events. Returns events in forward
// order for the caller to re-apply.
func (l *EventLog) Redo(count int) []any {
	var result []any
	pos := l.cursor
	redone := 0

	for pos < len(l.events) && redone < count {
		entry := l.events[pos]
		if !isCheckpoint(entry) {
			result = append(result, entry)
			redone++
		}
		pos++
	}

	l.cursor = pos
	return result
}

// Recent returns the last count non-checkpoint events (up to cursor)
// in chronological order (oldest first). If count is 0, returns all.
func (l *EventLog) Recent(count int) []any {
	limit := count
	if limit == 0 {
		limit = l.cursor
	}
	var result []any
	for i := l.cursor - 1; i >= 0 && len(result) < limit; i-- {
		entry := l.events[i]
		if !isCheckpoint(entry) {
			result = append(result, entry)
		}
	}
	// Reverse to get chronological order
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// Cursor returns the current cursor position (one past last applied event).
func (l *EventLog) Cursor() int {
	return l.cursor
}

// Length returns the total number of entries in the log (including checkpoints).
func (l *EventLog) Length() int {
	return len(l.events)
}

// CanUndo returns whether there are events before the cursor that can be undone.
func (l *EventLog) CanUndo() bool {
	for i := l.cursor - 1; i >= 0; i-- {
		if !isCheckpoint(l.events[i]) {
			return true
		}
	}
	return false
}

// CanRedo returns whether there are events after the cursor that can be redone.
func (l *EventLog) CanRedo() bool {
	return l.cursor < len(l.events)
}
