package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Feature 004 (T018, FR-003) : après un refus PodSecurity, l'analyse locale d'explain désigne
// le securityContext de chaque conteneur cité et propose la version corrigée, pour les seuls
// champs nommés par le serveur.
func TestPodSecurityFix(t *testing.T) {
	stderr, err := os.ReadFile("../../testdata/errors/apply-pod-security/stderr.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, violations, ok := strings.Cut(string(stderr), `violates PodSecurity "restricted:latest": `)
	if !ok {
		t.Fatalf("relevé inattendu : %s", stderr)
	}
	r := Analyze([]string{"../../testdata/objects/scenarios/pss-violation.yaml"}, false, "v1.37.2")
	r.AddPodSecurity(violations)
	var found []Issue
	for _, is := range r.Issues {
		if is.Kind == KindPodSecurity {
			found = append(found, is)
		}
	}
	if len(found) != 1 {
		t.Fatalf("un problème pod-security attendu (conteneur web) : %+v", r.Issues)
	}
	is := found[0]
	if is.Line != 14 || is.FieldPath != "spec.containers[0].securityContext" || !strings.Contains(is.Detail, "web") {
		t.Errorf("position ou champ inattendus : ligne %d, %s, %q", is.Line, is.FieldPath, is.Detail)
	}
	var fixed []string
	for _, l := range is.Fixed {
		fixed = append(fixed, l.Text)
	}
	got := strings.Join(fixed, "\n")
	for _, want := range []string{"capabilities:", `drop: ["ALL"]`, "seccompProfile:", "type: RuntimeDefault"} {
		if !strings.Contains(got, want) {
			t.Errorf("version corrigée sans %q :\n%s", want, got)
		}
	}
	// runAsNonRoot n'est pas cité par le serveur (déjà présent au niveau du pod) : pas ajouté.
	if strings.Contains(got, "runAsNonRoot") {
		t.Errorf("champ non cité ajouté :\n%s", got)
	}
}

// Un securityContext en ligne ({…}) est réécrit sur une seule ligne ; un conteneur sans
// securityContext en reçoit un, à l'indentation de ses champs.
func TestPodSecurityFixForms(t *testing.T) {
	dir := t.TempDir()
	src := "apiVersion: v1\nkind: Pod\nmetadata:\n  name: web\nspec:\n  containers:\n  - name: a\n    image: busybox:1.38\n    securityContext: {allowPrivilegeEscalation: false}\n  - name: b\n    image: busybox:1.38\n"
	path := filepath.Join(dir, "pod.yaml")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	r := Analyze([]string{path}, false, "v1.37.2")
	r.AddPodSecurity(`unrestricted capabilities (containers "a", "b" must set securityContext.capabilities.drop=["ALL"])`)
	want := map[int][]string{
		9:  {`    securityContext: {allowPrivilegeEscalation: false, capabilities: {drop: ["ALL"]}}`},
		10: {"  - name: b", "    securityContext:", "      capabilities:", `        drop: ["ALL"]`},
	}
	n := 0
	for _, is := range r.Issues {
		if is.Kind != KindPodSecurity {
			continue
		}
		n++
		var got []string
		for _, l := range is.Fixed {
			got = append(got, l.Text)
		}
		if strings.Join(got, "\n") != strings.Join(want[is.Line], "\n") {
			t.Errorf("ligne %d : version corrigée\n%s\nattendu\n%s", is.Line, strings.Join(got, "\n"), strings.Join(want[is.Line], "\n"))
		}
	}
	if n != 2 {
		t.Errorf("deux problèmes attendus, %d", n)
	}
}
