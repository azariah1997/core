package indexer

import "testing"

func TestBuildDispatchIndexesUserEvents(t *testing.T) {
	d, err := buildDispatch("user.updated", []byte(`{"id":"u1","displayName":"Alice","status":"active"}`))
	if err != nil {
		t.Fatalf("build dispatch: %v", err)
	}
	if d.action != actionIndex || d.doc.Type != "user" || d.doc.ID != "u1" {
		t.Fatalf("unexpected dispatch: %+v", d)
	}
	if d.doc.Fields["displayName"] != "Alice" || d.doc.Fields["status"] != "active" {
		t.Fatalf("unexpected fields: %+v", d.doc.Fields)
	}
}

func TestBuildDispatchDeletesOnUserDeleted(t *testing.T) {
	d, err := buildDispatch("user.deleted", []byte(`{"id":"u1"}`))
	if err != nil {
		t.Fatalf("build dispatch: %v", err)
	}
	if d.action != actionDelete || d.delType != "user" || d.delID != "u1" {
		t.Fatalf("unexpected dispatch: %+v", d)
	}
}

func TestBuildDispatchIndexesApplicationEvents(t *testing.T) {
	d, err := buildDispatch("application.created", []byte(`{"id":"app1","slug":"demo","name":"Demo","status":"active"}`))
	if err != nil {
		t.Fatalf("build dispatch: %v", err)
	}
	if d.action != actionIndex || d.doc.Type != "application" || d.doc.Fields["slug"] != "demo" {
		t.Fatalf("unexpected dispatch: %+v", d)
	}
}

func TestBuildDispatchIndexesMessageSent(t *testing.T) {
	d, err := buildDispatch("message.sent", []byte(`{"id":"m1","conversationId":"c1","senderId":"u1","type":"TEXT"}`))
	if err != nil {
		t.Fatalf("build dispatch: %v", err)
	}
	if d.action != actionIndex || d.doc.Type != "message" || d.doc.Fields["conversationId"] != "c1" {
		t.Fatalf("unexpected dispatch: %+v", d)
	}
}

func TestBuildDispatchSkipsUnrecognizedEventTypes(t *testing.T) {
	d, err := buildDispatch("relationship.created", []byte(`{"id":"r1"}`))
	if err != nil {
		t.Fatalf("build dispatch: %v", err)
	}
	if d.action != actionSkip {
		t.Fatalf("expected an unrecognized event type to be skipped, got %+v", d)
	}
}

func TestBuildDispatchRejectsMalformedPayload(t *testing.T) {
	if _, err := buildDispatch("user.updated", []byte(`not json`)); err == nil {
		t.Fatal("expected an error for malformed payload")
	}
}
