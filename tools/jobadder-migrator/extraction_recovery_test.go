package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestQueuedRequestsCarryOnlyTheirOwnIdentifiers(t *testing.T) {
	routes := routesFor("/companies/{companyId}", "/companies/fields/custom/{fieldId}")
	var companyRoute ExtractRoute
	for _, r := range routes {
		if r.Path == "/companies/{companyId}" {
			companyRoute = r
		}
	}
	task, err := makeTask(1, companyRoute, map[string]string{"companyId": "9", "candidateId": "7", "fieldId": "3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(task.Bindings) != 1 || task.Bindings["companyId"] != "9" {
		t.Fatal("unrelated routing context carried into another entity", task.Bindings)
	}
	v, _ := decodePage([]byte(`{"companyId":9,"name":"Synthetic company"}`))
	index := indexPage(task, companyRoute, v, routes)
	for _, queued := range index.Tasks {
		if strings.Contains(queued.Path, "/fields/custom/3") {
			t.Fatal("candidate field ID reused as company field ID")
		}
	}
}

func TestExtractionCheckpointsAndObjectsSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock(testPass); err != nil {
		t.Fatal(err)
	}
	id, err := s.BeginExtraction("source", false)
	if err != nil {
		t.Fatal(err)
	}
	routes := routesFor("/candidates")
	if err := s.EnqueueTasks(seedTasks(id, routes)); err != nil {
		t.Fatal(err)
	}
	task, err := s.ClaimTask(id)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := s.PutBlob(context.Background(), strings.NewReader("original before interruption"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Unlock(testPass); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginExtraction("source", true); err != nil {
		t.Fatal(err)
	}
	resumed, err := s.ClaimTask(id)
	if err != nil || resumed.ID != task.ID || resumed.Attempts != 1 {
		t.Fatal("running task not recovered", resumed, err)
	}
	if err := s.verifyBlob(context.Background(), blob.ID); err != nil {
		t.Fatal("durable original lost", err)
	}
}

func TestEightyThousandSourceRowsAreIndexedWithoutChangingIDs(t *testing.T) {
	s := testStore(t)
	var csv strings.Builder
	csv.WriteString("candidateId,notes\n")
	for i := int64(0); i < 80000; i++ {
		fmt.Fprintf(&csv, "%d,Original wording\n", 9007199254740993+i)
	}
	count, err := s.indexSourceRows(context.Background(), 1, strings.NewReader(csv.String()), ".csv", "candidates", "")
	if err != nil || count != 80000 {
		t.Fatal("80k source index incomplete", count, err)
	}
	var stored int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM source_rows WHERE source_id IN ('9007199254740993','9007199254820992')`).Scan(&stored); err != nil || stored != 2 {
		t.Fatal("boundary IDs changed", stored, err)
	}
}

func TestRecordIndexKeepsFullAndReferenceObservations(t *testing.T) {
	s := testStore(t)
	id, _ := s.BeginExtraction("source", false)
	for _, raw := range []string{`{"candidateId":7,"custom":[{"fieldId":3,"value":"keep"}]}`, `{"candidateId":7,"firstName":"Synthetic"}`} {
		b, err := s.PutBlob(context.Background(), strings.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.commitPage(ExtractTask{Export: id}, b, PageIndex{Records: []RecordRef{{"candidates", "7", ""}}}, nil, 1, 0); err != nil {
			t.Fatal(err)
		}
	}
	var observations int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM extract_records WHERE entity='candidates' AND source_id='7'`).Scan(&observations); err != nil || observations != 2 {
		t.Fatal("full record replaced with short reference", observations, err)
	}
	summary, err := s.ExtractionStatus()
	if err != nil || summary.Records != 1 {
		t.Fatal("observations counted as distinct source IDs", summary, err)
	}
}

func TestUnlinkedObjectsRemainInExportAndChangedPlansAreRejected(t *testing.T) {
	a := testApp(t)
	b, err := a.store.PutBlob(context.Background(), strings.NewReader("saved before association checkpoint"))
	if err != nil {
		t.Fatal(err)
	}
	report, err := a.store.extractionReport()
	if err != nil || report.UnlinkedObjects != 1 {
		t.Fatal("unlinked evidence not visible", report.UnlinkedObjects, err)
	}
	parts, err := a.store.exportParts()
	if err != nil || len(parts) != 1 || parts[0].Files != 1 {
		t.Fatal("unlinked evidence omitted from archive", parts, err)
	}
	objects, err := a.store.exportObjects(parts[0].First, parts[0].Last)
	if err != nil || len(objects) != 1 || objects[0].ID != b.ID {
		t.Fatal("original object missing", objects, err)
	}
	old := exportPlanKey(parts)
	if _, err := a.store.PutBlob(context.Background(), strings.NewReader("later response")); err != nil {
		t.Fatal(err)
	}
	w := call(a, "POST", "/api/download-ticket", `{"manifest":true,"part":0,"plan":"`+old+`"}`, true)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "changed") {
		t.Fatal("inconsistent archive plan accepted", w.Body.String())
	}
}
