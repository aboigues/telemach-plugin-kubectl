package pitfall

import (
	"errors"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
)

// ErrNotYet signale un type d'objet dont le diagnostic n'est pas encore disponible.
var ErrNotYet = errors.New("type d'objet pas encore pris en charge")

// Context regroupe ce dont un diagnostic d'objet a besoin.
type Context struct {
	Reader  Reader
	Scope   kuberead.Scope
	Catalog *catalog.Catalog
	Now     time.Time
}

// diagnosers associe à chaque type d'objet sa fonction de diagnostic ; chaque fichier de
// diagnostic y ajoute la sienne.
var diagnosers = map[Kind]func(ctx Context, name string) (ObjectSnapshot, error){}

// Diagnose diagnostique un objet. L'erreur renvoyée est celle de la lecture de l'objet lui-même
// (introuvable, cluster injoignable) ; les lectures annexes refusées rendent seulement le
// diagnostic partiel.
func Diagnose(ctx Context, kind Kind, name string) (ObjectSnapshot, error) {
	d, ok := diagnosers[kind]
	if !ok {
		return ObjectSnapshot{}, ErrNotYet
	}
	return d(ctx, name)
}

// newSnapshot prépare l'instantané d'un objet et les valeurs communes de ses leçons.
func newSnapshot(ctx Context, kind Kind, name string) ObjectSnapshot {
	return ObjectSnapshot{
		Kind: kind, Name: name, Namespace: ctx.Scope.Namespace, Context: ctx.Scope.Context,
		Captures: map[string]string{
			"kubectl": "kubectl", "scope": ctx.Scope.Flags(), "name": name, string(kind): name,
			"namespace": namespaceLabel(ctx.Scope),
		},
	}
}

func namespaceLabel(s kuberead.Scope) string {
	if s.Namespace != "" {
		return s.Namespace
	}
	return "courant"
}
