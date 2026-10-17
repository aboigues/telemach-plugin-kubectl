// Package pitfall diagnostique les pièges silencieux (feature 002) : un Service, un
// Deployment, un Ingress ou un PVC créé sans erreur mais qui ne fonctionne pas. Il suit les
// maillons d'un objet dans l'ordre où un formateur les vérifierait, en lecture seule (toutes
// les lectures passent par internal/kuberead), et ne présente jamais comme cassé un maillon
// qu'il n'a pas pu lire.
package pitfall
