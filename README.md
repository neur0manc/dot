# dotmgr

config management akin to yadm. no-dependency, single, static binary
for linux, windows and macos.

## installation

for now the releases are built on builds.sr.ht, but artifacts are
removed after 90 days. have to look into make the download /
installation convenient. so if destiny is on your side you might
find a current release on this projects builds.sr.ht site.

## usage

`dotmgr help`

## banter

### q: why make this when yadm is great?

yadm really is great, used it for years. but

- it's not natively available on windows
- it requires git
- (and python)

and i personally am a 'single-static-binary-no-deps'-enjoyer which
is a thing.

### q: but go?

go really is great indeed

- native git library available
- cross-compiling on alpine linux for all platforms
- no giant .net installation for compiling on windows
- honestly i also just like the syntax
- 50 mb binary size for a simple hello world seem to be solved as well
  (although that was never that big of a deal but it's nice)

but most important: it has a good, cute mascot

### disclaimer

i played product manager in this project, the role of eager junior
software engineer was played by a certain farting cat, named 5.6 sol.
