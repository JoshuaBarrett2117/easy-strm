package controller

import "testing"

func TestNormalizeDeleteFileIDs(t *testing.T) {
	tests := []struct {
		name     string
		fileIDs  []string
		fileID   string
		filePath string
		want     []string
	}{
		{
			name:    "prefers explicit list",
			fileIDs: []string{"a", "b"},
			fileID:  "single",
			want:    []string{"a", "b"},
		},
		{
			name:   "falls back to file_id",
			fileID: "single",
			want:   []string{"single"},
		},
		{
			name:     "falls back to file_path",
			filePath: "dir/movie.mkv",
			want:     []string{"dir/movie.mkv"},
		},
		{
			name: "returns nil when empty",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeDeleteFileIDs(tt.fileIDs, tt.fileID, tt.filePath)
			if len(got) != len(tt.want) {
				t.Fatalf("unexpected length: got %d want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("unexpected value at %d: got %q want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

