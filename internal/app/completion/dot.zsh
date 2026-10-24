#compdef dot
# zsh completion for dot. Load it with: source <(dot completion zsh)

_dot() {
	local -a candidates
	candidates=("${(@f)$(dot __complete "${(@)words[2,CURRENT]}" 2>/dev/null)}")

	if [[ ${candidates[1]} == :files ]]; then
		_files
		return
	fi

	local -a values others
	values=(${(M)candidates:#*=})
	others=(${candidates:#*=})

	(( ${#values} )) && compadd -Q -S '' -a values
	(( ${#others} )) && compadd -Q -a others
}

if (( $+functions[compdef] )); then
	compdef _dot dot
fi
