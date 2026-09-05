<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# Documentation for connect project

[Protocol description](./protocol_description.md) shows message types and their sequence in process of connection to server.


## Entities that i need to think about

- World representation
    - representation of world for lonely bot outside of any other bots vision
    - representation of world for bot in party, but outside of other party members vision
    - representation of world for bot in party and nearby


- goroutines for handling bots activities
    - connection to server
    - receiving data from game server
    - sending data to game server
    - handling ping form server
    - handling packets from server (update in bot game state representation)
    - handling packets form server (event driven logic)
    - bot logic
    - party logic
    - clan logic
    - alliance logic
    - cli commands channel
    - gui commands channel
    - gui representation channel
    - multiple l2 clients connections
    - ?

## Conventions

### Logging
- Each output starts from capital letter
- Periods not used at end of line
- Errors start with "Error"
- Only use \n at end of output text in case of Printf, otherwise use Println