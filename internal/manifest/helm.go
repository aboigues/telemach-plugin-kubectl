package manifest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Fichiers d'un chart Helm (issue #12) : ce ne sont pas des manifestes à vérifier tels quels.

// helmDirective reconnaît une directive propre à Helm : objets .Values, .Release, .Chart…,
// ou fonctions include, define, template. Les gabarits des alertes Prometheus
// ({{ $labels.instance }}, {{ $value }}) ne sont pas concernés : ce sont des chaînes YAML
// valides.
var helmDirective = regexp.MustCompile(`\{\{-?\s*(?:[^}]*\.(?:Values|Release|Chart|Capabilities|Files|Template)\b|(?:include|define|template)\s)`)

// isHelmTemplate indique si le fichier est un template Helm : rangé sous templates/ d'un
// dossier qui contient Chart.yaml, ou contenant une directive propre à Helm.
func isHelmTemplate(f *File) bool {
	if templatesChart(f.Path) != "" {
		return true
	}
	for _, l := range f.Lines {
		if helmDirective.MatchString(l) {
			return true
		}
	}
	return false
}

// helmTemplateIssue signale un template Helm, à la première ligne qui contient « {{ ».
func helmTemplateIssue(f *File) (Issue, bool) {
	if !isHelmTemplate(f) {
		return Issue{}, false
	}
	is := Issue{Kind: KindHelmTemplate, File: f.Path, Line: 1, Column: 1, Suggestion: chartOrPlaceholder(templatesChart(f.Path))}
	for i, l := range f.Lines {
		if c := strings.Index(l, "{{"); c >= 0 {
			is.Line, is.Column = i+1, len([]rune(l[:c]))+1
			break
		}
	}
	return is, true
}

// chartPlaceholder remplace le dossier du chart quand il n'est pas connu.
const chartPlaceholder = "CHEMIN-DU-CHART"

// chartOrPlaceholder renvoie le dossier du chart, préfixé par « ./ » s'il est relatif : sans
// lui, Helm pourrait chercher un chart de dépôt du même nom.
func chartOrPlaceholder(dir string) string {
	switch {
	case dir == "":
		return chartPlaceholder
	case filepath.IsAbs(dir) || strings.HasPrefix(dir, "."):
		return dir
	}
	return "./" + dir
}

// templatesChart renvoie le dossier du chart (tel que désigné par -f) dont path est rangé
// sous templates/, ou "".
func templatesChart(path string) string {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		if filepath.Base(dir) == "templates" && isChartDir(filepath.Dir(dir)) {
			return filepath.Dir(dir)
		}
		if dir == filepath.Dir(dir) || dir == "." {
			return ""
		}
	}
}

func isChartDir(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "Chart.yaml")) //nolint:gosec // dossier désigné par -f, simple test d'existence en lecture seule
	return err == nil
}

// helmValuesIssue signale un fichier de valeurs (values.yaml, traefik-values.yaml…) ou la
// description d'un chart (Chart.yaml) : aucun de ses documents ne déclare de kind, et son
// nom l'annonce. Detail précise lequel des deux.
func helmValuesIssue(f *File) (Issue, bool) {
	base := strings.ToLower(filepath.Base(f.Path))
	detail := ""
	switch {
	case base == "chart.yaml":
		detail = "Chart.yaml"
	case strings.Contains(base, "values"):
		detail = "values"
	default:
		return Issue{}, false
	}
	for _, d := range f.Documents {
		if d.Kind != "" || (detail == "values" && d.APIVersion != "") {
			return Issue{}, false
		}
	}
	chart := ""
	if isChartDir(filepath.Dir(f.Path)) {
		chart = filepath.Dir(f.Path)
	}
	return Issue{Kind: KindHelmValues, File: f.Path, Line: 1, Column: 1, Detail: detail, Suggestion: chartOrPlaceholder(chart)}, true
}
