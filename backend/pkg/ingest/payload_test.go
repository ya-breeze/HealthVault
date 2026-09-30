package ingest_test

import (
	"encoding/json"
	"testing"

	"github.com/ya-breeze/healthvault/pkg/ingest"
)

func TestPayloadJSON_TestFlagRequiresJSONBoolean(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    ingest.TestFlag
		wantErr bool
	}{
		{name: "missing defaults false", body: `{}`, want: false},
		{name: "false allows normal ingestion", body: `{"test":false}`, want: false},
		{name: "true marks synthetic payload", body: `{"test":true}`, want: true},
		{name: "null rejected", body: `{"test":null}`, wantErr: true},
		{name: "number rejected", body: `{"test":1}`, wantErr: true},
		{name: "string rejected", body: `{"test":"true"}`, wantErr: true},
		{name: "array rejected", body: `{"test":[]}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ingest.PayloadJSON
			err := json.Unmarshal([]byte(tt.body), &got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Unmarshal error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got.Test != tt.want {
				t.Errorf("Test = %v, want %v", got.Test, tt.want)
			}
		})
	}
}
