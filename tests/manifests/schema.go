package manifests

// SchemaCheck valide un manifeste avec les règles Kubernetes embarquées (trois versions). Elle
// est complétée par la tâche T058, quand le paquet internal/schema existe ; d'ici là, seule la
// syntaxe YAML est contrôlée (par Check).
func SchemaCheck(_ []byte) []string { return nil }
