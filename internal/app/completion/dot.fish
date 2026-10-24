# fish completion for dot. Load it with: dot completion fish | source

function __dot_complete
	set -l candidates (dot __complete (commandline -opc)[2..-1] (commandline -ct) 2>/dev/null)

	if test "$candidates[1]" = :files
		__fish_complete_path (commandline -ct)
	else
		printf '%s\n' $candidates
	end
end

complete -c dot -f -a '(__dot_complete)'
