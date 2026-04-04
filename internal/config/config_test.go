package config

import "testing"

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg:  Config{Services: map[string][]int{"docmost": []int{3000}, "filebrowser": []int{80}}},
		},
		{
			name:    "empty services rejected",
			cfg:     Config{},
			wantErr: true,
		},
		{
			name:    "empty service name rejected",
			cfg:     Config{Services: map[string][]int{"": []int{3000}}},
			wantErr: true,
		},
		{
			name:    "missing allowed ports rejected",
			cfg:     Config{Services: map[string][]int{"docmost": nil}},
			wantErr: true,
		},
		{
			name:    "port range enforced",
			cfg:     Config{Services: map[string][]int{"docmost": []int{70000}}},
			wantErr: true,
		},
		{
			name:    "duplicate ports rejected",
			cfg:     Config{Services: map[string][]int{"docmost": []int{3000, 3000}}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := Config{Services: map[string][]int{"docmost": []int{3000}}}
	cfg.applyDefaults()

	if cfg.ListenAddr != defaultListenAddr {
		t.Fatalf("ListenAddr = %q, want %q", cfg.ListenAddr, defaultListenAddr)
	}
	if cfg.LogPath != defaultLogPath {
		t.Fatalf("LogPath = %q, want %q", cfg.LogPath, defaultLogPath)
	}
}
