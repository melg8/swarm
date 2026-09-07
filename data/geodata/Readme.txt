##############################################
GEODATA PACK OF THIS REPOSITORY
##############################################

Complete old-world L2J geodata (the C1 continent): 165 region files
X_Y.l2j covering the region grid 16_10 .. 26_26, the l2j headerless
format the Mobius C1 server and the swarm pathfind engine read.

Source: INTERLUDE/Geodata2/geodata of the LGK-Games/Geodata GitHub
mirror, every file sha1-verified against the upstream git tree at
download time. The 21_19.l2j region (Elven Village surroundings) is
byte identical with the file this project already used, which verified
the pack against the running server before the bulk import.

The bot picks this directory automatically (first geodata candidate of
cmd/swarm); the server keeps its own subset in
dist/game/data/geodata.

##############################################
GEODATA COMPENDIUM (upstream readme)
##############################################

Comprehensive guide for geodata.

How to configure it
        a - Prerequisites
        b - Make it work

##############################################
How to configure it
##############################################

----------------------------------------------
a - Prerequisites
----------------------------------------------

* A 64bits Windows/Java JDK is a must-have to run server with geodata. Linux servers don't have the issue.
* The server can start (hardly) with -Xmx3000m. -Xmx4g is recommended.

----------------------------------------------
b - Make it work
----------------------------------------------

To make geodata work:
* unpack your geodata files into "/data/geodata" folder (or any other folder)
