<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# Goal
Main goal of this project is to give me motivation to learn and apply programming languages while having in mind development of concrete application with special requirements from botting l2 subject area.

Currently i haven't made clear choice of final language of implementation. Main considirations are: golang looks promising with support of goroutines out of the box, but might be not best in case of need to optimize every part of bot for support of large number of connections at same time from not that powerfull pc. Other language might be C++, which i have at least development experience with. And know that it can be finetuned to greater extend than go. But golang has simplicity on its pros as well. So i might go back and forth on this in future. Maybe middle ground would be like critical paths implemented in C++ for higher speeds, and main asynchronous part of bots implemented in golang.

# Project description
Current project is software written in golang that works as out of game (OOG) multi-instance proxy botting software. It should support multiple bots connected to game server at same time from single go program.

## Properties

- User can start one or more instances of program at same time with differrent configuration files, they should work without colliding with eachother. Example: main instance runs multiple bots at same time performing long term run, while second instance is used to login into test characters, and perform short runs for development/debugging purposes.

- Program should support ability of bots to reconnect to server in case of lost connection.

- When connecting any number of bots into server (>1) bots should not connect in random order, but should be ordered on some criteria (for example class priority). Example: tanks login first, then healers, then rest of party.

- Program should support clean gracefull shutdown with all bot instances exiting game world and finishing their respective goroutines on signal from user at any time of run.

- Program should support three different way of user communication with it: 
    - console input in bot server terminal
    - gui programm displaying bots statuses and giving way to affect their behavior (desktop app or in browser type of gui).
    - one or more lineage 2 c4 clients connected in proxy mode to one of characters controlled by server's bot.

- Program should support large number of connected bots at same time from single instance: 
    - Minimal target requirement is 9 concurrent bots from single instance. (one party of bots per processor).
    - Optimistic requirement 36 (4x9) concurrent bots at same time. (One party per core).
    - Overly optimistic 72 (8x9) concurrent bots at same time. (One party per hyperthread).
    - Probably not realistic 100 (11*10 + 1) concurrent bots. (More then one party per hyperthread).
Exact numbers that can be achieved under my current pc spec and with  programming technologies used will be determined in process of development.

- To achieve previously stated properties program must have a mechanism for saving computing resources in the case when many bots are in the same visibility zone and filter identical packets coming from the server about entities in the bots visibility zone. 
Scenario: siege with 100 concurrent bots. If bots are close to eachother (within sight of eachother) and command is send to move 100 bots closer to castle doors, it will trigger server to broadcasting this 100 moves back to 100 bot clients. Idea is to somehow filter this 10k (100x100) messages into more bearable load. One possibility is using concept of "eyes" - single bot instance gets all information and handles it, while rest of bots just ignore messages from server, and only rely on state that is gathered by "eyes" bot. Other solution would be to have some time window of gathering packets from server, for entity which already had packet in that time window rest of packets will be marked as duplicates and ignored.

- Bot structure should support programming different high level scenarios of bots actions:
    - fully independend actions of bots, probably in differenet locations (like 1-20 solo leveling/questing/proffesion change).
    - semi-synchronized actions like party gathering into single "unit" together from different places, party going from town into farming spot entrance etc.
    - syncrhonized actions - like party farming in same spot, with actions logically adjusted to actions of other party members. (Not aggroing mobs while tank is not aggroed them first, changing focus target in case of aggresion on weak members of party, heals of party members etc.)
    - hyper synchronized actions of party (think of ability to implement aoe party farm in open spaces or in catachombs). Where some members of party should stay in some place, while others should go gather enemies, healers should wait for aoe farmers to gather aggression, before starting healing process etc. This type of scenario more on wishfull thinking of my abilites to implement, but i want at least to design system to be in principle able to allow such things.
    - syncrhonized actions on large scale - like multiple parties working together to farm raid bosses, or even alliance of bots performing a siege of castle (this would be an ultimate goal). With ability of cross party interactions (like healers performing heals on bots from other paries by knowing their actual health status etc.)

- As some "fun" type of bot activies would be:
    - making pictures with dropping gold for dithered png picture.
    - sync actions of bots in movements - i would like bots aesthetics be less of herd of animals just following single party member (like l2walker or l2net bots do that) but more coherent movements of characters with predefined relative positions of bots in party, and ability to sync bots with different speeds so they dont split up on long distance runs.
    - large number of bots sync actions like applied emotions with maybe syncrhonized soulshot effects. Just for cinematic view.
    - large number of bots moving into some different formations for screenshot type of stuff (like lines of warriors, mages, healers etc).

- Program should support fully authonomous run from console, without any need of real person actions. So in principle it would be possible to have CI setup to run server, and then run bots on it and check their performance in different situations for some time.

- Program should work on local hosted l2c4 l2jlisvus emulator, with all anitcheat/antibot settings enabled in c4 only communication mode. And without any custom changes to emulator code.

- Program should not rely on any of its bots having non "user" type of right on server. So no "gm" mode bots.

- Currently all bot logic supposed to be implemented in go language as well and statically linked into final application. So no support of bot scripts in different language. Probably no even hot reload of bots logic, but i will need to look into this more closely, maybe some support of hot reloading of go code or alternatively - behavior code as microservice with reloading it wihtout losing connection of bot to game might be possible.


## Concrete examples of what should be possible
1. Alot of separate characters farming on their own 1-21 lvl for example.
2. Small groups of character farming together like 2-3 characters in same location.
3. Full party farming together as synchronized unit of work.
4. Several parties together (3 for example) farming minibosses.
5. 8 or more parties participating together in epic boss or castle battle.

Uknowns but assumed:
1. thread should be able to hold and calculate up to 9 characters at time.


Ideas:
1. Most of time (from 21 till 78) leveling process is in party setting.
2. Which of data can be shared between different clients
3. Which of computations can be eliminated while preserving bots behavior.

Questions
1. How detalized i want bot management be on larger scales?


Fun epic movements:
1. All 9 of characters emote attack and engage soulshots when their hand in top most position.

Usefull synchronization:
Burst damage - melee get close to target, ranges get into attack range. Casters begin to cast spells, in moment of spell hitting target - mele also hit target.