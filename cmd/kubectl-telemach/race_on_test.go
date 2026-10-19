//go:build race

package main

// raceEnabled indique une compilation avec le détecteur de courses (-race), qui ralentit
// l'exécution d'un facteur 5 à 10 : les objectifs de latence ne s'y mesurent pas.
const raceEnabled = true
