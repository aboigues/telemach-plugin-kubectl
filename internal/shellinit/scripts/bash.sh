# Intégration TELEMACH pour bash — à activer avec :
#   eval "$(kubectl-telemach shell-init bash)"
# Explique en français les commandes kubectl en échec. Garanties (contracts/cli.md) : stdin et
# stdout ne sont jamais redirigés, le code de sortie d'origine est toujours renvoyé, aucune
# autre commande n'est observée, aucun alias n'est créé.

# Binaire kubectl-telemach, à l'emplacement où il était installé lors de la génération.
__telemach_bin=__TELEMACH_BIN__

# Intégration active et version du plugin qui l'a générée : « kubectl telemach last » repère
# ainsi un terminal sans intégration, ou chargé par une version antérieure (D-LL-013).
export TELEMACH_KUBECTL_SHELL=__TELEMACH_SHELL__

# Vérification différée (FR-028 à FR-031) : après une création réussie (apply, create, run),
# « kubectl-telemach watch » vérifie en arrière-plan, environ 10 s plus tard, le seul statut
# des pods concernés ; son signal est déposé dans __telemach_pending et affiché à l'invite
# suivante, jamais pendant une saisie. Seulement dans un shell interactif.
__telemach_pending=__TELEMACH_PENDING__
__telemach_shell_pid=$$
__telemach_watch_on=0
[[ $- == *i* ]] && __telemach_watch_on=1

# Lance la vérification sans attendre ni rien afficher (entrées et sorties sur /dev/null).
__telemach_watch() {
	[[ "$__telemach_watch_on" == 1 && "${TELEMACH_WATCH:-}" != "0" ]] || return 0
	command "$__telemach_bin" watch --shell-pid "$__telemach_shell_pid" -- "$@" </dev/null >/dev/null 2>&1 &
}

# Crochet d'invite : affiche puis supprime les signaux de ce shell ; sans signal, aucun
# programme n'est lancé. Le code de retour de la dernière commande est préservé.
__telemach_prompt() {
	local __telemach_status=$? __telemach_f="$__telemach_pending/pending-$__telemach_shell_pid"
	if [[ -s "$__telemach_f" ]]; then
		printf '%s\n' "$(<"$__telemach_f")" >&2
		command rm -f -- "$__telemach_f"
	fi
	return $__telemach_status
}
if [[ "$__telemach_watch_on" == 1 ]]; then
	# PROMPT_COMMAND peut être une chaîne ou (bash ≥ 5.1) un tableau ; ajout idempotent.
	if [[ "$(declare -p PROMPT_COMMAND 2>/dev/null)" == "declare -a"* ]]; then
		[[ " ${PROMPT_COMMAND[*]} " == *" __telemach_prompt "* ]] || PROMPT_COMMAND+=(__telemach_prompt)
	else
		case ";${PROMPT_COMMAND:-};" in
		*";__telemach_prompt;"*) ;;
		*) PROMPT_COMMAND="__telemach_prompt${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;;
		esac
	fi
fi

# Commande kubectl réelle (research.md R4). Si « kubectl » est un alias (« microk8s kubectl »,
# « minikube kubectl -- »), son texte est découpé en mots par le shell, sans être évalué ; un
# rechargement de ce script garde la commande déjà trouvée.
__telemach_alias_kubectl=0
if [[ -n "${BASH_ALIASES[kubectl]+x}" ]]; then
	__telemach_alias_kubectl=1
	if [[ "${BASH_ALIASES[kubectl]}" != __telemach_kubectl* ]]; then
		read -ra __telemach_kubectl_cmd <<<"${BASH_ALIASES[kubectl]}"
	fi
fi
if [[ ${#__telemach_kubectl_cmd[@]} -eq 0 ]]; then
	__telemach_kubectl_cmd=(kubectl)
fi
# « kubectl » en premier mot désignerait la fonction ou l'alias de ce script : on vise le
# programme lui-même.
if [[ "${__telemach_kubectl_cmd[0]}" == kubectl ]]; then
	__telemach_kubectl_cmd=(command "${__telemach_kubectl_cmd[@]}")
fi
if [[ "${__telemach_kubectl_cmd[0]}" == command ]]; then
	export TELEMACH_KUBECTL="${__telemach_kubectl_cmd[*]:1}"
else
	export TELEMACH_KUBECTL="${__telemach_kubectl_cmd[*]}"
fi

# Version du client kubectl, lue une seule fois (lecture locale, sans contact avec le cluster).
__telemach_kubectl_version="$("${__telemach_kubectl_cmd[@]}" version --client -o json 2>/dev/null </dev/null |
	sed -n 's/.*"gitVersion": *"\([^"]*\)".*/\1/p' | head -n 1)"
export TELEMACH_KUBECTL_VERSION="$__telemach_kubectl_version"

# Exécute kubectl en captant stderr (sous-shell : les pièges restent locaux). Premier argument :
# le nom tapé par l'apprenant (« kubectl » ou « k »), pour écrire les corrections.
__telemach_run() (
	__telemach_invoked=$1
	shift
	if [ "${TELEMACH_DISABLE:-}" = "1" ] || [ "${1:-}" = "telemach" ] || ! command -v "$__telemach_bin" >/dev/null 2>&1; then
		"${__telemach_kubectl_cmd[@]}" "$@"
		exit $?
	fi

	# Sous-commande (premier mot hors options globales) et commande interactive ou de longue
	# durée (research.md R3) : stderr n'est alors pas redirigé, pour ne pas perturber le
	# terminal.
	__telemach_verb=""
	__telemach_skip=0
	__telemach_i=0
	__telemach_t=0
	__telemach_follow=0
	__telemach_watch=0
	for __telemach_arg in "$@"; do
		if [ "$__telemach_skip" = 1 ]; then
			__telemach_skip=0
			continue
		fi
		case "$__telemach_arg" in
		--) break ;;
		-n | --namespace | --context | --kubeconfig | --cluster | --user | -s | --server | --token | --as | --as-group | --as-uid | --request-timeout | -v | --v | --certificate-authority | --client-certificate | --client-key | --tls-server-name | --cache-dir | --username | --password | --profile | --profile-output | --log-flush-frequency | --vmodule | -c | --container | -f | --filename | -o | --output | -l | --selector)
			[ "$__telemach_arg" = -f ] && [ "$__telemach_verb" = logs ] && __telemach_follow=1 && continue
			__telemach_skip=1
			;;
		-it | -ti) __telemach_i=1 && __telemach_t=1 ;;
		-i | --stdin | --stdin=true) __telemach_i=1 ;;
		-t | --tty | --tty=true) __telemach_t=1 ;;
		--follow | --follow=true) __telemach_follow=1 ;;
		-w | --watch | --watch=true | --watch-only | --watch-only=true) __telemach_watch=1 ;;
		-*) ;;
		*) [ -z "$__telemach_verb" ] && __telemach_verb=$__telemach_arg ;;
		esac
	done
	# exec, attach, debug et port-forward gardent leur terminal (stdin et stdout intacts) mais
	# leur stderr est capté : leurs erreurs (conteneur introuvable, transfert refusé) ne viennent
	# que de kubectl (research.md S10).
	__telemach_interactive=0
	__telemach_capture=0
	case "$__telemach_verb" in
	attach | port-forward) __telemach_capture=1 ;;
	edit | proxy) __telemach_interactive=1 ;;
	exec | debug) { [ "$__telemach_i" = 1 ] || [ "$__telemach_t" = 1 ]; } && __telemach_capture=1 ;;
	run) { [ "$__telemach_i" = 1 ] || [ "$__telemach_t" = 1 ]; } && __telemach_interactive=1 ;;
	logs) [ "$__telemach_follow" = 1 ] && __telemach_interactive=1 ;;
	get | events) [ "$__telemach_watch" = 1 ] && __telemach_interactive=1 ;;
	esac

	if [ "$__telemach_interactive" = 1 ]; then
		"${__telemach_kubectl_cmd[@]}" "$@"
		__telemach_rc=$?
		if [ "$__telemach_rc" -ne 0 ]; then
			command "$__telemach_bin" explain --exit-code "$__telemach_rc" --interactive \
				--kubectl-version "$__telemach_kubectl_version" --invoked-as "$__telemach_invoked" -- "$@" || true
		else
			__telemach_after_success "$@"
		fi
		exit "$__telemach_rc"
	fi

	# Pièges installés avant la création du fichier : une interruption (Ctrl-C) à n'importe
	# quel moment ne laisse aucun fichier temporaire derrière elle (D-LL-003).
	__telemach_tmp=""
	trap '[ -n "$__telemach_tmp" ] && rm -f -- "$__telemach_tmp" "$__telemach_tmp.rc"' EXIT
	trap 'exit 130' INT
	trap 'exit 143' TERM
	__telemach_tmp="$(umask 077 && mktemp "${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}/telemach-kubectl.XXXXXX")" || {
		"${__telemach_kubectl_cmd[@]}" "$@"
		exit $?
	}

	# stderr de kubectl passe par tee (copie dans le fichier temporaire) ; stdout reste intact
	# grâce au descripteur 3.
	if [ "$__telemach_capture" = 1 ]; then
		# Session interactive : Ctrl-C revient à kubectl. Le sous-shell continue pour expliquer une
		# erreur captée avant l'interruption, et tee, qui ignore Ctrl-C, garde tout ce que kubectl
		# a écrit. Le code de kubectl est noté par le pipeline lui-même : après un trap, celui
		# du pipeline n'est plus fiable en zsh.
		trap ':' INT
		{ {
			trap ':' INT
			"${__telemach_kubectl_cmd[@]}" "$@" 2>&1 1>&3 3>&-
			echo "$?" >"$__telemach_tmp.rc"
		} | (trap '' INT && exec tee -- "$__telemach_tmp") >&2; } 3>&1
		__telemach_rc=$(cat -- "$__telemach_tmp.rc" 2>/dev/null)
		[ -n "$__telemach_rc" ] || __telemach_rc=130
	else
		{ "${__telemach_kubectl_cmd[@]}" "$@" 2>&1 1>&3 3>&- | tee -- "$__telemach_tmp" >&2; } 3>&1
		__telemach_rc=${PIPESTATUS[0]}
	fi

	if [ "$__telemach_rc" -ne 0 ]; then
		command "$__telemach_bin" explain --exit-code "$__telemach_rc" --stderr-file "$__telemach_tmp" \
			--kubectl-version "$__telemach_kubectl_version" --invoked-as "$__telemach_invoked" -- "$@" || true
	else
		__telemach_after_success "$@"
	fi
	exit "$__telemach_rc"
)

# Après une création réussie, la vérification différée est lancée ; c'est « watch » qui écarte
# les commandes non concernées (--dry-run, -f -, URL, -k). « expose » crée un Service ;
# « scale » peut voir ses pods refusés par un quota (feature 004). Un avertissement PodSecurity
# d'une commande réussie est expliqué tout de suite par « notice » : le fichier est lu par une
# boucle « read », intégrée au shell, sans lancer de processus (research.md F5).
__telemach_after_success() {
	case "$__telemach_verb" in
	apply | create | run | expose | scale) __telemach_watch "$@" ;;
	esac
	[ -s "${__telemach_tmp:-}" ] || return 0
	local __telemach_line
	while IFS= read -r __telemach_line || [ -n "$__telemach_line" ]; do
		case "$__telemach_line" in
		*"would violate PodSecurity"*)
			command "$__telemach_bin" notice --stderr-file "$__telemach_tmp" -- "$@" || true
			return 0
			;;
		esac
	done <"$__telemach_tmp"
}

__telemach_kubectl() { __telemach_run kubectl "$@"; }
__telemach_kubectl_as() { __telemach_run "$@"; }

# Interception de « kubectl » : par l'alias s'il en existait un, sinon par une fonction. Une
# fonction « kubectl » définie par l'utilisateur n'est jamais remplacée.
if [[ "$__telemach_alias_kubectl" == 1 ]]; then
	alias kubectl='__telemach_kubectl'
elif declare -F kubectl >/dev/null && [[ "$(declare -f kubectl)" != *__telemach_kubectl* ]]; then
	printf '%s\n' "TELEMACH : une fonction « kubectl » existe déjà dans ce shell ; elle n'est pas remplacée et les erreurs de kubectl ne seront pas expliquées automatiquement." >&2
else
	# « function » : le nom n'est pas développé comme un alias à la lecture du bloc.
	function kubectl { __telemach_kubectl "$@"; }
fi

# Alias « k » : couvert seulement s'il désigne déjà kubectl (FR-001a) ; jamais créé.
if [[ -n "${BASH_ALIASES[k]+x}" ]]; then
	__telemach_k_words=()
	read -ra __telemach_k_words <<<"${BASH_ALIASES[k]}"
	case "${__telemach_k_words[0]:-}" in
	kubectl | */kubectl) BASH_ALIASES[k]="__telemach_kubectl_as k${BASH_ALIASES[k]#"${__telemach_k_words[0]}"}" ;;
	esac
	unset __telemach_k_words
fi
