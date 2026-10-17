package textdist

import "testing"

func TestDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"pods", "pods", 0},
		{"pdos", "pods", 2},
		{"deploymnet", "deployment", 2},
		{"service", "services", 1},
		{"", "abc", 3},
		{"créer", "creer", 1},
	}
	for _, tt := range tests {
		if got := Distance(tt.a, tt.b); got != tt.want {
			t.Errorf("Distance(%q, %q) = %d, attendu %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestClosest(t *testing.T) {
	types := []string{"pods", "deployments", "services", "configmaps"}
	if got, ok := Closest("pdos", types, 2); !ok || got != "pods" {
		t.Errorf("Closest(pdos) = %q, %v ; attendu pods", got, ok)
	}
	if _, ok := Closest("persistentvolumeclaims", types, 2); ok {
		t.Error("aucune suggestion attendue pour un mot éloigné")
	}
	if _, ok := Closest("pods", types, 2); ok {
		t.Error("aucune suggestion attendue pour un mot déjà correct")
	}
}

func TestTypoDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"pdos", "pods", 1},
		{"deploymnet", "deployment", 1},
		{"pod", "pod", 0},
		{"", "abc", 3},
		{"gt", "get", 1},
	}
	for _, tt := range tests {
		if got := TypoDistance(tt.a, tt.b); got != tt.want {
			t.Errorf("TypoDistance(%q, %q) = %d, attendu %d", tt.a, tt.b, got, tt.want)
		}
	}
}
