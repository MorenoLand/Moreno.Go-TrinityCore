# Next4 source ownership and acceptance matrix

This matrix is based on the pinned TrinityCore checkout at `dcdbc0c5d88eb96f412f69c34bd5b9de2eed5df6`, the current Go tree, and executable evidence paths. Historical local test evidence was removed from the public tree by policy; source compilation or a self-check is not a behavioral acceptance claim.

| No. | Pinned reference owner | Go owner | Current evidence | Acceptance |
| ---: | --- | --- | --- | --- |
| 1 | repository commit, source inventory, and Next3 reconciliation | `docs/PARITY.md`, `docs/PARITY_COVERAGE.md`, Next4 evidence log | baseline and pinned commit recorded | recorded |
| 2 | all source areas listed by `src/server`, `src/tools`, `src/server/scripts`, and `tests` | this matrix plus `tools/parity`, `tools/scriptinventory` | every numbered owner is listed here | matrix complete; behavior open |
| 3 | `src/server/game/Server/WorldSocket.cpp`, `WorldSession.cpp`, and packet handlers | `docs/NEXT4_PACKET_STATE_REPLAY.md`, `pkg/protocoltrace`, `tools/packettrace`, `engine/world/server.go`, `engine/auth/trace.go`, `engine/service/service.go`, `engine/database/database.go` | source-defined JSON packet replay, failure replay, and redacted database transition results pass | complete for source-defined replay; client/login state evidence remains in items 4–5 |
| 4 | `Handlers/CharacterHandler.cpp`, `Entities/Player/Player.cpp::SendInitialPacketsBeforeAddToMap/AfterAddToMap`, `Maps/Map.cpp::SendInitSelf/AddPlayerToMap`, `Maps/Map.cpp::GetZoneAndAreaId` | `engine/world/characters.go`, `worldstates.go`, `terrain.go`, `player_packets.go`, `player_state.go`, `transports.go`, `death.go`, `skills.go`, `guild.go`, `dynamic_spells.go`, `tools/logincheck`, `tools/terraincheck`, `tools/worldstatecheck` | source-defined login order, single visibility pass (`879320e`), deterministic talent order (`f505235`), attached/map transport create phases (`35f3d8d`), attached transport passenger create batch (`5a30b16`), player GUID low-word update-mask correction and source-order follow-up (`901c330`), `PLAYER_CHOSEN_TITLE` field correction (`3307faa`), public player update-field flag correction (`939d6fb`), create-block field verifier (`c8dbd96`), mandatory achievement/equipment login-order stages (`1b0473a`), map 530/zone 3430 world-state fixture (`708486c`), hidden-duration aura packet correction (`a0a25e6`), live-aura hidden-duration correction (`0a9d5e9`), persisted pet-aura hidden-duration correction (`49f74ab`), aura effect-mask correction (`19dedcb`), runtime aura effect-index correction (`080f4e6`), single aura-update effect-index correction (`bb64c14`), packed achievement timestamps (`5dad1ad`), post-load achievement completion timing (`cb7dfeb`), hidden achievement visibility (`fc28f25`), creature cast-speed/hover-height fields (`bca9cf4`), creature query fallback correction (`1c4343c`), transport dynamic path progress (`10ca358`), transport route period (`2e078a9`), transport passenger hover height (`bc57f4d`), supplied September login trace correlation (pre-verify `SMSG_CRITERIA_UPDATE`), DBC reputation-list mapping (`286d282`), active pet create fields (`79e5df9`), pet ActiveStates preservation (`47cd644`), guild roster state (`0c74b76`), source login time speed (`a47bf9d`), regression verifier for pre-verify achievement packets (`b1deed4`), inventory create-field boundary (`e9e239b`), packed login time (`8506995`), ready rune resync (`118cd98`), triggered login effect (`c69879c`), cinematic branch (`50caa6b`), time-sync schedule (`a3ccee0`), guild bank login packet (`ba58ec7`), guild signed-on event (`8fe0b3b`), guild MOTD event (`6a88a62`), saved player appearance fields and source-shaped empty guild-event parameters (`80ed6b9`), nearby-player public visibility (`278c4bb`), reciprocal player create broadcast (`351037c`), stateful player visibility/removals (`0c2de4e`), dynamic spell object login visibility (`a7206c4`), nearby corpse/bones visibility (`2397643`), recorded login order/create-block verifier (`ef02ca3`), difficulty, cooldown, unlearn, movement, corpse, terrain/WMO area, and world-state fixtures | open: complete source-defined packet/update-mask comparison |
| 5 | `Entities/Player/Player.cpp::LoadFromDB`, `_LoadSkills`, `_LoadSpells`, `_LoadInventory`, `_LoadAuras`, `_LoadQuestStatus` | `engine/world/player_state.go`, `player_packets.go`, `player_auras.go`, `characters.go`, `items.go`, `quests.go`, `skills.go`, `pets.go` | focused equipment, spell, language, glyph/aura, rested, real-time item-duration, cinematic, pet-cooldown, reset tests; inventory create-field boundary (`e9e239b`); persisted pet summon-effect replay (`4ea1f9b`); summoned-pet owner-level normalization (`a938e7e`) | open: complete character-state diff |
| 6 | `Handlers/LogoutHandler.cpp`, `Entities/Player/Player.cpp::SaveToDB`, `WorldSession` teardown | `engine/world/characters.go`, `server.go`, `player_state.go` | logout and persistence tests | open: full normal logout packet/database fixture |
| 7 | logout combat, taxi, battleground, disconnect, and character-switch branches in `LogoutHandler.cpp`, `WorldSession.cpp` | `engine/world/characters.go`, `server.go`, `battleground.go`, `taxi.go` | targeted lifecycle tests | open: complete edge-case matrix |
| 8 | `auth/AuthSocket.cpp`, `Handlers/AuthSession.cpp`, `WorldSocket.cpp` | `engine/auth`, `engine/crypto`, `server/authserver` | SRP, reconnect, version, ban, and duplicate-session tests | open: result/disconnect parity matrix |
| 9 | `Spells/Spell.cpp`, `SpellEffects.cpp`, `Spells/Auras/*`, `Unit.cpp` | `engine/world/spells.go`, `spell_stats.go`, `dynamic_spells.go`, `player_auras.go` | cast, GCD, interrupt, resistance, aura, and target tests | open: DBC-backed resource/aura fixtures |
| 10 | `Entities/Creature/Creature.cpp`, `Entities/Unit/Unit.cpp`, `Combat/ThreatManager.cpp`, AI owners | `engine/world/combat.go`, `creaturemotion.go`, `threat.go`, `kill.go`, `pet_combat.go` | deterministic threat, combat, evade, and death tests | open: full target-switch and persistence parity |
| 11 | `Movement/*`, `Entities/Unit/Unit.cpp`, `Maps/Transport.cpp`, `TransportMgr.cpp` | `engine/world/movement.go`, `transports.go`, `position.go` | source-defined transport-relative GUID/offset, taxi, attached/map transport login creates (`35f3d8d`), cross-map transfer-pending/new-world packets, and post-worldport map-entry phases (`d63dfac`) | open: live boat/zeppelin spline and client-compatibility verification |
| 12 | `Handlers/LootHandler.cpp`, `Loot/Loot.cpp`, corpse and map ownership paths | `engine/world/loot.go`, `kill.go`, `death.go` | loot, money, group, corpse, and release tests; nearby corpse visibility (`2397643`) | open: complete eligibility/persistence replay |
| 13 | `Handlers/NPCHandler.cpp`, `Entities/Item/Item.cpp`, vendor/buyback state | `engine/world/vendors.go`, `items.go`, `characters.go` | vendor and buyback fixtures | open: unlimited/limited stock and relog parity |
| 14 | `Handlers/GossipHandler.cpp`, `NPCHandler.cpp`, `GameObject.cpp`, spirit-healer and transport handlers | `engine/world/gossip.go`, `npc_interaction.go`, `gameobjects.go`, `death.go`, `transports.go` | gossip, NPC, gameobject, spirit, and reduced-schema tests | open: valid/rejected/ghost/GM client stability |
| 15 | `Handlers/ChatHandler.cpp`, `Chat/Channel.cpp`, `Chat/Chat.cpp` | `engine/world/chat.go`, `channels.go`, `social.go` | source-defined chat/channel and encrypted chat fixtures | open: all channel/language/addon routing verification |
| 16 | `Groups/*`, `Guilds/*`, `Trade/*`, `Mail/*`, `AuctionHouse/*`, calendar and equipment-set owners | `engine/world/group.go`, `guild.go`, `trade.go`, `mail.go`, `auctionhouse.go`, `calendar.go`, `items.go` | focused state-machine tests by subsystem; source-order guild login bank packet (`ba58ec7`), signed-on event (`8fe0b3b`), MOTD event (`6a88a62`), and roster bank rights (`57f4140`) | open: full normal/failure persistence matrix |
| 17 | pinned Moreno custom scripts/config and custom command owners | `engine/world/lfg.go`, `npcbots.go`, `commands.go`, `engine/scripting`, `docs/PARITY.md` | custom foundations and targeted tests | open: enabled/disabled fixture parity |
| 18 | `Battlegrounds/*`, `Arena/*`, instance/timer owners | `engine/world/battleground*.go`, `arena*.go` | historical battleground and arena lifecycle evidence | open: async timer/race and packet parity |
| 19 | `database/Implementation/*Database.cpp` prepared-statement registration | `engine/database/statements_generated.go`, `statement_registry.go`, `tools/dbtool` | identifier inventory and SQLite execution audit | open: all parameter/result/transaction cases |
| 20 | `sql/*`, `database/Database/DatabaseLoader.cpp`, update/migration owners | `sql/mysql`, `sql/sqlite`, `engine/database/migrations.go`, `bootstrap.go` | schema inventory and migration tests | open: complete dialect/locking/update matrix |
| 21 | all `src/server/scripts` loaders and `AddSC_` registrations | `engine/scripting`, `tools/scriptinventory`, `docs/SCRIPT_PARITY_MANIFEST.md` | 709-file/registration inventory | open: every module implementation/equivalent |
| 22 | `game/LuaEngine/*`, Eluna bindings and local scripts | `engine/scripting`, `engine/world/lua_*.go`, `lua_scripts` runtime paths, `tools/luacheck` | all 12 shipped scripts load, Lua string pattern probes pass (`c35f8b4`), and normal PlayerEvent 18 chat delivery is not cancelled (`46aada6`) | open: Lua/Eluna registration and behavior coverage |
| 23 | `game/Server/Protocol/Opcodes.cpp` and all handler owners | `pkg/protocol/opcodes_generated.go`, `engine/world/server.go`, `tools/parity` | 727 registrations; 38 static source references | open: per-opcode runtime evidence |
| 24 | `src/tools/map_extractor` MPQ/archive owners | `tools/mpq` | parser and compression unit fixtures | open: real MPQ output/error parity |
| 25 | `src/tools/map_extractor` WDT/ADT/DBC/liquid owners | `tools/mapextractor` | source and synthetic tests | open: real fixture/output parity |
| 26 | `src/tools/vmap4_extractor`, `vmap4_assembler` | `tools/vmap4extractor`, `tools/vmap4assembler` | geometry and assembly unit fixtures | open: WMO/M2/VMAP047 fixture parity |
| 27 | `src/tools/mmaps_generator` Recast/Detour owners | `tools/mmaps-generator` | scaffold/header tests | open: actual tile generation and query parity |
| 28 | release/build/test owners across reference and Go repositories | `scripts`, `go.mod`, `docs`, remote `main` | signed pushes, source compilation, vet, build; race environment blocked | open until 1–27 acceptance gates pass |

The first open behavioral gate remains the complete source-defined login packet/update-mask/state comparison in items 4–5; later rows are not promoted by inventory counts.

The login regression verifier includes an executable self-check (`go run ./tools/logincheck -self-check`) for the pre-`SMSG_LOGIN_VERIFY_WORLD` achievement-packet ordering rule (`a8c2896`).

Active pets now use a runtime object GUID separate from the persistent pet number, matching the pinned `Pet::LoadPetFromDB` identity boundary (`80b5fbd`).

Login aura movement now emits the standalone water-walk, feather-fall, and hover packets before the compound movement state, matching `Player::SendInitialPacketsAfterAddToMap` (`44f92c3`).

The same login aura handlers now broadcast full movement-state packets to nearby clients, matching `Player::SetWaterWalking`, `SetFeatherFall`, `SetHover`, and `SetCanFly` (`69d173b`).

Nearby login movement packets now retain transport-relative GUID/offset/seat data, matching `Unit::BuildMovementPacket` for attached players (`7894416`).

Mounted-flight aura login now recalculates and broadcasts flight speed packets through the source `UpdateSpeed(MOVE_FLIGHT)` branch (`c60e325`).

Persisted transform auras now resolve their creature-template display before the player create update, matching `_LoadAuras` and `HandleAuraTransform` (`2ad2911`).

Special transform spells now use the pinned race/gender display mappings for login serialization (`13d8503`).

Persisted mounted auras now resolve mount creature display models before player create serialization, matching `HandleAuraMounted` (`a10722a`).

Holiday mount spell 62061 now selects the pinned flying/ground creature display variant during login mount reconstruction (`9afa4ee`).

Login zone state now derives sanctuary, capital, hostile-area, PvP, FFA, arena, and resting flags from AreaTable and realm configuration before the initial world-state packet (`8ce2915`).

The same login state now applies sanctuary and arena flags from the resolved area entry, matching `UpdateArea` as well as `UpdateZone` (`b9164ed`).

Persisted aura loading now drops unknown spell rows before client serialization, matching `_LoadAuras` validation (`c0b7734`).

Persisted pet health and mana are now clamped to the source-computed maxima before active-pet create and pet-spell packets, matching `Pet::LoadPetFromDB` (`a4da0fc`, `528fd18`).

Login movement replay now follows `SendInitialPacketsAfterAddToMap`: direct water-walk/feather-fall/hover, flight enable/speed, stun root, compound movement, then aura visibility; `tools/logincheck` rejects the inverse order (`ef27ee6`).

Zone and area transitions now reapply the same sanctuary, PvP, arena, hostile-area, and resting state logic used at login before sending the player update (`daafbde`).

The parity inventory was refreshed against the pinned reference after the movement/state changes; it reports 727 registered opcodes, 612 prepared statements, and 0 static opcode or SQL-identifier gaps, while retaining the explicit behavioral-evidence caveat (`docs/PARITY_COVERAGE.md`).

The login/movement zone path now removes persisted auras whose `Spell.dbc` `RequiredAreasID` no longer contains the resolved zone or area through `AreaGroup.dbc`, and evaluates `spell_area` quest, aura, race, gender, and autocast requirements (`607fa1d`, `1fb90d6`).

Zone changes now trigger the loaded group update after local-channel and exploration processing, matching `Player::UpdateZone` group-update flag behavior (`4ea7c8e`).

Character enumeration and login now validate race/class/gender/skin/face/hair/facial-hair combinations against `CharSections.dbc` and `CharacterFacialHairStyles.dbc`, sanitizing enumeration and rejecting invalid login state like `Player::ValidateAppearance` (`6638654`).

Zone hostility now includes active `quest_template.Flags & QUEST_FLAGS_PVP` quests, matching `Player::HasPvPForcingQuest` (`478cbbd`).

PvP-forcing quest hostility is now preserved through faction-group evaluation instead of being overwritten by the zone branch (`019b63d`).

Zone dynamic weather now loads `game_weather`, follows TrinityCore’s seasonal regeneration/intensity thresholds, emits `SMSG_WEATHER` on login/zone entry, and broadcasts changes on the world tick with configurable activation/interval (`3980bc3`).

Alive zone transitions now delete `item_template.Map`/`area`-limited inventory items, despawn their client GUIDs, and refresh inventory/equipment fields, matching `Player::DestroyZoneLimitedItem` (`6b68159`).

Friendly-zone PvP state now retains and expires TrinityCore’s five-minute PvP-off timer instead of immediately losing/recreating the flag on zone transitions (`dd69a94`).

Player zone transitions now invoke the Eluna-compatible `PLAYER_EVENT_ON_UPDATE_ZONE` hook with the resolved zone and area (`b184fc9`).

Login map entry and cross-map teleports now invoke the Eluna-compatible `PLAYER_EVENT_ON_MAP_CHANGE` hook (`645f2ec`).

Achievement state now loads before player initialization and login criteria/stat evaluation, matching `Player::LoadFromDB` ordering (`61b1518`).

Zone transitions and login now auto-unequip invalid offhands using learned Dual Wield/Titan Grip state and relocate the item to a free backpack slot, matching `AutoUnequipOffhandIfNeed` (`822480c`).

Zone transitions now persist the live zone and guild roster serialization uses online members’ current zone, matching `Guild::UpdateMemberData` (`b1a3257`).

Wintergrasp zone enter/leave transitions now invoke the existing battlefield state hooks during login and movement, matching `BattlefieldWG::OnPlayerEnterZone/OnPlayerLeaveZone` (`969866d`).

Persisted pets now replay TrinityCore's `CreatedBySpellId` `SMSG_SPELL_GO` with the owner as caster before the pet create block, matching `Pet::LoadPetFromDB`'s summon-effect path (`4ea1f9b`).

Permanent summoned pets now use the owner level during login before pet stats, health/mana clamping, and create serialization, matching `Pet::LoadPetFromDB`'s `SUMMON_PET` branch (`a938e7e`).

Cross-map teleports now send `SMSG_TRANSFER_PENDING`, preserve transport-relative `SMSG_NEW_WORLD` coordinates, defer map-change hooks until `MSG_MOVE_WORLDPORT_ACK`, and replay the source map-entry self/inventory/transport/visibility/aura/status phases (`d63dfac`).
