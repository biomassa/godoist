package tui

import (
	"testing"

	"github.com/biomassa/godoist/internal/state"
)

func TestNotesMarker(t *testing.T) {
	if got := withNotesMarker("About this project", true); got != "About this project\n\ngodoist:notes" {
		t.Errorf("add = %q", got)
	}
	if got := withNotesMarker("About\n\ngodoist:notes", false); got != "About" {
		t.Errorf("remove = %q", got)
	}
	if !hasNotesMarker("x\n godoist:notes ") || hasNotesMarker("godoist:notesx") {
		t.Error("hasNotesMarker is wrong")
	}
}

// v writes the marker to the project description and removes the local setting.
func TestToggleNotesWritesMarker(t *testing.T) {
	m := openWork(t)
	m.ui.ProjectModes["p1"] = state.ModeNotes
	m = send(t, m, key("v")) // notebook → tasks
	if m.notesMode() || hasNotesMarker(m.projects["p1"].Description) || m.pending != 1 {
		t.Fatalf("after v: notes = %v desc = %q pending = %d", m.notesMode(), m.projects["p1"].Description, m.pending)
	}
	if _, ok := m.ui.ProjectModes["p1"]; ok {
		t.Error("the local setting is still there")
	}
	m = send(t, m, key("v")) // tasks → notebook
	if !m.notesMode() || !hasNotesMarker(m.projects["p1"].Description) || m.pending != 2 {
		t.Errorf("after v v: notes = %v desc = %q pending = %d", m.notesMode(), m.projects["p1"].Description, m.pending)
	}
}

// The first sync writes the local notebook settings to Todoist. When the marker comes
// back from Todoist, the local setting goes.
func TestMoveNotesModes(t *testing.T) {
	m := testModel(t)
	m.ui.ProjectModes["p1"] = state.ModeNotes
	m.moveNotesModes()
	if m.pending != 1 || !m.notesMoved {
		t.Fatalf("pending = %d, want one write", m.pending)
	}
	m.moveNotesModes() // a second sync in this run does not write again
	if m.pending != 1 {
		t.Errorf("pending = %d after the second sync", m.pending)
	}
	m.projects["p1"].Description = notesMarker
	m.moveNotesModes()
	if _, ok := m.ui.ProjectModes["p1"]; ok {
		t.Error("the local setting stayed after Todoist has the marker")
	}
}
