# bash completion for dot. Load it with: source <(dot completion bash)

_dot() {
	local line=${COMP_LINE:0:COMP_POINT}
	local -a words
	read -ra words <<<"$line"
	[[ $line == *[[:space:]] ]] && words+=("")

	local cur=${words[${#words[@]}-1]}
	local IFS=$'\n'
	local -a candidates
	candidates=($(dot __complete "${words[@]:1}" 2>/dev/null))

	if [[ ${candidates[0]} == :files ]]; then
		compopt -o filenames 2>/dev/null
		COMPREPLY=($(compgen -f -- "$cur"))
		return
	fi

	# bash replaces only the text after the last word break, such as the
	# "=" in --os=linux, so drop what comes before it.
	local strip=${cur%"${COMP_WORDS[COMP_CWORD]}"}
	COMPREPLY=("${candidates[@]#"$strip"}")

	if [[ ${#COMPREPLY[@]} == 1 && ${COMPREPLY[0]} == *= ]]; then
		compopt -o nospace 2>/dev/null
	fi
}

complete -F _dot dot
