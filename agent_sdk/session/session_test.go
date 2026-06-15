package session

import (
	"testing"
)

func TestSessionStateSnapshotIsVersionedAndTolerant(t *testing.T) {
	full := SessionState{
		Summary:     "s",
		SkillsInUse: []string{"k"},
		Memory:      map[string]any{"seq": 1, "long": []any{}},
	}.ToJSON()
	if full["v"] != SnapshotVersion {
		t.Errorf("v = %v", full["v"])
	}
	if _, ok := full["memory"]; !ok {
		t.Error("memory missing from snapshot")
	}

	// unknown future key ignored; missing keys default
	withFuture := map[string]any{}
	for k, v := range full {
		withFuture[k] = v
	}
	withFuture["_future"] = 1
	st := SessionStateFromJSON(withFuture)
	if len(st.SkillsInUse) != 1 || st.SkillsInUse[0] != "k" {
		t.Errorf("skills_in_use = %v", st.SkillsInUse)
	}

	legacy := SessionStateFromJSON(map[string]any{"summary": "legacy"})
	if len(legacy.Memory) != 0 {
		t.Errorf("legacy memory = %v", legacy.Memory)
	}
}

func TestTurnRoundTrip(t *testing.T) {
	tn := Turn{Role: "user", Content: "hello", Metadata: map[string]any{"k": "v"}}
	j := tn.ToJSON()
	if j["role"] != "user" || j["content"] != "hello" {
		t.Errorf("turn json = %v", j)
	}
	back := TurnFromJSON(j)
	if back.Role != "user" || back.Content != "hello" {
		t.Errorf("round-trip = %+v", back)
	}
	msg := tn.ToMessage()
	if msg["role"] != "user" || msg["content"] != "hello" {
		t.Errorf("to_message = %v", msg)
	}
}

func TestSessionStateMessagesShortVerbatim(t *testing.T) {
	st := SessionState{History: []Turn{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}}
	msgs := st.Messages(1, 6, 2000)
	if len(msgs) != 2 || msgs[0]["role"] != "user" || msgs[0]["content"] != "hello" {
		t.Errorf("messages = %v", msgs)
	}
}

func TestSessionStateMessagesLongShaped(t *testing.T) {
	var hist []Turn
	for i := 0; i < 10; i++ {
		hist = append(hist, Turn{Role: "user", Content: "m"})
	}
	st := SessionState{History: hist, Summary: "prior summary"}
	msgs := st.Messages(1, 3, 2000)
	// first message is the rolled-up conversation block
	first, _ := msgs[0]["content"].(string)
	if first == "" || msgs[0]["role"] != "user" {
		t.Errorf("first block = %v", msgs[0])
	}
	// last 3 turns kept natively
	if len(msgs) != 4 {
		t.Errorf("messages len = %d (%v)", len(msgs), msgs)
	}
}
