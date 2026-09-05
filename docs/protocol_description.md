<!--
SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT
-->

# Description of client-server protocol

Description client-server communication protocol based on [L2JLisvus](https://gitlab.com/TheDnR/l2j-lisvus/) server emulator.


# Authentification

Game servers have at least two differrent revisions of auth server protocol c621 and 785a. l2j-lisvus uses c621 which is described below.

```mermaid
    sequenceDiagram
    participant Client
    participant Auth Server
    
    autonumber 0
    Client->>+Auth Server: Tcp/Ip connect
    activate Auth Server

    alt new ip
    Auth Server-->>Client: id:0 Init
    else same ip flood
    autonumber 2
    Auth Server->>Client: Tcp/Ip disonnect
    end

    deactivate Auth Server

    Client->>Auth Server: id:7 RequestGGAuth
    activate Auth Server
    Auth Server-->>Client: id:11 GGAuth 
    deactivate Auth Server
    
    #Client--o--o Auth Server: Blowfish encryption on
    Client->>Auth Server: id:0 RequestAuthLogin
    activate Auth Server

    alt good credentials
        Auth Server-->>Client: id:3 LoginOk 
    else bad credentials
        autonumber 6
        Auth Server-->>Client: id:1 LoginFail
    end
    
    deactivate Auth Server

    Client->>Auth Server: id:5 RequestServerList
    activate Auth Server
    Auth Server-->>Client: id:4 ServerList
    deactivate Auth Server

    Client->>Auth Server: id:2 RequestServerLogin
    activate Auth Server

    alt server ok
        Auth Server-->>Client: id:7 PlayOk 
    else server busy
        autonumber 10
        Auth Server-->>Client: id:6 PlayFail
    end
    deactivate Auth Server
    
    Client->>Auth Server: Tcp/Ip disconnect
```

Process of handling recieved packets:
1. Check recieved raw data length and compare it with expected packet length from data[2] field.
2. If raw data is smaller then expected length, read tcp/ip again and concatenate data
3. Decrypt packet if packet expected to be encrypted
4. Check packet checksum, discard packet if checksum is invalid
5. Check packet type, discard packet if type is not expected in current state of authentification
6. Deserialize body of packet into concrete data structure
7. Use data structure in next state of authentification
    

## Auth server -> client packets


### 0. Packets common structure
Packets have similar structure to eachother. They consist of:
   

| Hex | Size | Bytes | Enc | Description |
|-----|------|-------|-----|-------------|
|XX XX|2|[0-1]| |Size of packet|
|XX |1|[2]| 🔓 |Id of packet|
|XX XX XX XX .. |N|[3-(N+2)]| 🔓|Body of packet|
|00 .. |0-4|[(N+3)-(N+6)]| 🔓|Padding with zeroes for 8 byte alignment of packet|
|XX XX XX XX |4|[(N+7)-(N+10)]| 🔓| Checksum (only auth server communications) |



There are 6 different types of data that can be passed in packet

| Hex | Size | Type description |
|-----|------|-------------|
|XX XX XX .. \0|N|string UTF8|
|XX XX XX XX ..|8|float|
|XX XX XX XX ..|8|int 64|
|XX XX XX XX|4|int 32|
|XX XX|2|int 16|
|XX|1|int 8|



Auth server packets are encrypted using [Blowfish](https://en.wikipedia.org/wiki/Blowfish_(cipher)) algorithm with 21 bytes hardcoded key:
```
5F 3B 35 2E 5D 39 34 2D 33 31 3D 3D 2D 25 78 54 21 5E 5B 24 # Actual key
00 # End of key indicator
```


## Auth server -> client packets


### 1. [Init](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/serverpackets/Init.java#L19)
----

| Hex | Size | Description | Bytes |
|-----|------|-------------|-------|
| 00 | 1 | [Type](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/serverpackets/Init.java#L43) | [0] |
| XX XX XX XX |  4 | [Session ID](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/serverpackets/Init.java#L44) | [1 - 4] |
| 21 C6 00 00| 4 | [Protocol revision](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/serverpackets/Init.java#L45) | [5 - 8] |
| XX XX XX XX ... | 128| [RSA Public Key](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/serverpackets/Init.java#L47)| [9 - 136] |
| 29 DD 95 4E | 4 | [GG related](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/serverpackets/Init.java#L50) | [137 - 140] |     
| 77 C3 9C FC | 4 | [GG related](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/serverpackets/Init.java#L51) | [141 - 144] |          
| 97 AD B6 20 | 4 | [GG related](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/serverpackets/Init.java#L52) | [145 - 148] |     
| 07 BD E0 F7 | 4 | [GG related](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/serverpackets/Init.java#L53) | [149 - 152] |
| XX XX XX XX ...| 20 | [Blowfish key (Only if compatibility mode enabled)](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/serverpackets/Init.java#L57) | [153 - 172] |
| 00 | 1 | [End of key indicator (Only if compatibility mode enabled)](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/serverpackets/Init.java#L58) | [173] |

Example of raw full Init packet, notice total length of packet is 159 bytes caused by 2 bytes with size of packet at beginning of packet and 4 bytes with padding at the end of packet. Server sends this packet unencrypted and without
checksum:
```diff
0000:|9f 00|00|b8 cd 6b 8b|21 c6 00 00|2e ac d4 98 2c  .....k.!.......,
0010: 71 bb 2f a7 6f 1d af 58 7c fd d3 c2 6d c4 f4 c4  q./.o..X|...m...
0020: 2b 9f 3b 09 42 c7 8c 72 e9 7d 03 bf 24 4b df d2  +.;.B..r.}..$K..
0030: 85 64 88 58 dc 8c f6 a6 ac 78 cc 75 5b ae 3d 7b  .d.X.....x.u[.={
0040: 18 ec 5e e1 d5 48 13 1c 00 63 d2 02 1f 35 f5 34  ..^..H...c...5.4
0050: 35 1d d6 90 8f 34 e3 3c d4 ed f9 83 55 b4 61 3b  5....4.<....U.a;
0060: 73 1d a2 4f 1b 3e 71 c3 af 0c 14 b8 1c 2f bf 64  s..O.>q....../.d
0070: 66 06 e5 77 4f 26 fe b8 da b3 2c c5 55 68 37 e5  f..wO&....,.Uh7.
0080: 9f 65 84 c3 93 71 0e 35 f3 15 59|4e 95 dd 29|fc  .e...q.5..YN..).
0090: 9c c3 77|20 b6 ad 97|f7 e0 bd 07|00 00 00 00     ..w ...........
Size: 159 bytes
```

Parsed Init packet:
```
InitPacket:
  SessionID: 8b6bcdb8
  ProtocolVersion: 0000c621
  RsaPublicKey:
    2eacd4982c71bb2fa76f1daf587cfdd3
    c26dc4f4c42b9f3b0942c78c72e97d03
    bf244bdfd285648858dc8cf6a6ac78cc
    755bae3d7b18ec5ee1d548131c0063d2
    021f35f534351dd6908f34e33cd4edf9
    8355b4613b731da24f1b3e71c3af0c14
    b81c2fbf646606e5774f26feb8dab32c
    c5556837e59f6584c393710e35f31559

  GameGuard1: 29dd954e
  GameGuard2: 77c39cfc
  GameGuard3: 97adb620
  GameGuard4: 07bde0f7
  BlowfishKey: nil
```


### 2. [GGAuth](https://gitlab.com/TheDnR/l2j-lisvus/-/blob/main/core/java/net/sf/l2j/loginserver/serverpackets/GGAuth.java#L24)

----

| Hex | Size | Description | Bytes |
|-----|------|-------------|-------|
| 0B | 1 | [Type](https://gitlab.com/TheDnR/l2j-lisvus/-/blob/main/core/java/net/sf/l2j/loginserver/serverpackets/GGAuth.java#L45) | [0] |
| XX XX XX XX |  4 | [Session ID](https://gitlab.com/TheDnR/l2j-lisvus/-/blob/main/core/java/net/sf/l2j/loginserver/clientpackets/RequestAuthGG.java#L81)| [1 - 4] |
| 00 00 00 00 |  4 | [Unknown](https://gitlab.com/TheDnR/l2j-lisvus/-/blob/main/core/java/net/sf/l2j/loginserver/serverpackets/GGAuth.java#L47)| [5 - 8] |


## Client -> auth server packets

### 1. [RequestGGAuth](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/clientpackets/RequestAuthGG.java#L23)


| Hex | Size | Description | Bytes |
|-----|------|-------------|-------|
| 07 | 1 | [Type](https://gitlab.com/TheDnR/l2j-lisvus/-/blob/main/core/java/net/sf/l2j/loginserver/L2LoginPacketHandler.java#L55) | [0] |
| XX XX XX XX | 4 | [Session ID](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/clientpackets/RequestAuthGG.java#L25) | [1 - 4] |
| 23 92 90 4D | 4 | [Data 1](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/clientpackets/RequestAuthGG.java#L26) | [5 - 8] |
| 18 30 B5 7C | 4 | [Data 2](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/clientpackets/RequestAuthGG.java#L27) | [9 - 12] |
| 96 61 41 47 | 4 | [Data 3](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/clientpackets/RequestAuthGG.java#L28) | [13 - 16] |
| 05 07 96 FB | 4 | [Data 4](https://gitlab.com/TheDnR/l2j-lisvus/-/blame/main/core/java/net/sf/l2j/loginserver/clientpackets/RequestAuthGG.java#L29) | [17 - 20] |


### 2. [RequestAuthLogin](https://gitlab.com/TheDnR/l2j-lisvus/-/blob/main/core/java/net/sf/l2j/loginserver/clientpackets/RequestAuthLogin.java#L31)
| Hex | Size | Description | Bytes |
|-----|------|-------------|-------|
| 00 | 1 | [Type](https://gitlab.com/TheDnR/l2j-lisvus/-/blob/main/core/java/net/sf/l2j/loginserver/L2LoginPacketHandler.java#L65) | [0] |
| 00 00 00 00 ... | 89 | [Padding](https://gitlab.com/TheDnR/l2j-lisvus/-/blob/main/core/java/net/sf/l2j/loginserver/clientpackets/RequestAuthLogin.java#L45) | [1 - 90] |
| 24 | 1 | [Account Start Flag](https://gitlab.com/TheDnR/l2j-lisvus/-/blob/main/core/java/net/sf/l2j/loginserver/clientpackets/RequestAuthLogin.java#L52) | [91] |
| 00 00 | 2 | [Padding](https://gitlab.com/TheDnR/l2j-lisvus/-/blob/main/core/java/net/sf/l2j/loginserver/clientpackets/RequestAuthLogin.java#L52) | [92 - 93] |
| XX XX XX XX ... | 14 | [Account](https://gitlab.com/TheDnR/l2j-lisvus/-/blob/main/core/java/net/sf/l2j/loginserver/clientpackets/RequestAuthLogin.java#L82) | [94 - 107] |
| XX XX XX XX ... | 16 | [Password](https://gitlab.com/TheDnR/l2j-lisvus/-/blob/main/core/java/net/sf/l2j/loginserver/clientpackets/RequestAuthLogin.java#L83) | [108 - 124] |
| 00 00 | 2 | [Padding]() | [125 - 127] |


## Steps of packet assembly
- RegisterNewPacket(packetID int32, packetSize int32)
    - reserve 2 bytes for packet size
    - set packet id to proper value for each packet
- Write...(...)
    - insert packet data
- Assemble()
    - reserve 0-7 bytes for padding depending on current size to be 8 byte aligned
    - calculate size of packet and insert to first 2 bytes
    - claculate checksum from packet id to end of packet - 4 bytes
    - insert checksum to last 4 bytes
    - encrypt from 2 to end
- Packet()



# Game server packets added by swarm

Reference implementation: the Mobius C1 sources
(`L2J_Mobius_C1_HarbingersOfWar/java/org/l2jmobius/gameserver`). Game packets
are framed by a 2 byte little endian size header followed by
`[opcode: 1][body]`, encrypted with the stateful XOR cipher, integers are
little endian.

## Client -> game server packets

### RequestActionUse (0x45)

The action button packet. Action id 0 toggles sit/stand; the server refuses
to sit while moving, casting or attacking (`RequestActionUse.runImpl` case 0,
`Player.sitDown`/`standUp`). The bot uses it to rest at low HP.

| Offset | Size | Field |
|--------|------|-------|
| 0 | 1 | Opcode 0x45 |
| 1 | 4 | Action id (0 = sit/stand toggle) |
| 5 | 4 | Ctrl pressed (0/1) |
| 9 | 1 | Shift pressed (0/1) |

### MoveToLocation (0x01)

The ground click movement request of the official client: the server walks
the character to the target point (`MoveToLocation.runImpl`). Mode 1 is the
mouse click; keyboard mode (0) is ignored unless keyboard movement is
enabled on the server. The hunt loop walks to a drop with it before
clicking the item.

| Offset | Size | Field |
|--------|------|-------|
| 0 | 1 | Opcode 0x01 |
| 1 | 4 | Target X |
| 5 | 4 | Target Y |
| 9 | 4 | Target Z |
| 13 | 4 | Origin X (client side position) |
| 17 | 4 | Origin Y |
| 21 | 4 | Origin Z |
| 25 | 4 | Movement mode (1 = mouse) |

## Game server -> client packets

### ChangeWaitType (0x3F)

Announces the sit/stand transition of a creature
(`ChangeWaitType.writeImpl`, enum values 0 = sitting, 1 = standing). The
broadcast goes through `Player.broadcastPacket`, so the acting client
receives its own transitions; it is the confirmation the hunt loop waits
for before ever sending the opposite toggle.

| Offset | Size | Field |
|--------|------|-------|
| 0 | 1 | Opcode 0x3F |
| 1 | 4 | Object id |
| 5 | 4 | Move type (0 = sitting, 1 = standing) |
| 9 | 4 | X |
| 13 | 4 | Y |
| 17 | 4 | Z |

### GetItem (0x17) position semantics

`GetItem` carries the position of the ITEM, not of the picker
(`Item.pickupMe` writes the item location; the official client only
animates the item flying into the inventory). The picker position is
already known from the StopMove broadcasts of the arrival, so the tracker
must not snap the picker to these coordinates - it teleported the marker
across the map on every pickup.
