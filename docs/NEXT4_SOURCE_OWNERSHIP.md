# Next4 source ownership and acceptance matrix

The 2026-09-22 closeout restores the source `UPDATEFLAG_HAS_TARGET` create payload for players with a victim and recipient-specific incremental `UF_FLAG_PARTY_MEMBER` quest IDs, including same-instance visibility filtering for quest, emote, and stat updates. Forward swim rates now apply the same source minimum/slow modifiers as the other serialized movement modes. A final `Unit::BuildValuesUpdate` comparison also restored cross-faction group faction/PvP fields for initial, incremental, and transport-passenger creates using the configured `AllowTwoSide.Interaction.Group` rule and `FactionTemplateEntry::IsFriendlyTo`. Login self-checks cover those recipient values plus update flags, party-only quest visibility, and movement modifier arithmetic. Item 4 is complete against its packet-order, update-mask, and post-map fixture gate; the row 4 `open` marker below is the earlier audit state.

This matrix is based on the pinned TrinityCore checkout at `dcdbc0c5d88eb96f412f69c34bd5b9de2eed5df6`, the current Go tree, and executable evidence paths. Historical local test evidence was removed from the public tree by policy; source compilation or a self-check is not a behavioral acceptance claim.

| No. | Pinned reference owner | Go owner | Current evidence | Acceptance |
| ---: | --- | --- | --- | --- |
| 1 | repository commit, source inventory, and Next3 reconciliation | `docs/PARITY.md`, `docs/PARITY_COVERAGE.md`, Next4 evidence log | baseline and pinned commit recorded | recorded |
| 2 | all source areas listed by `src/server`, `src/tools`, `src/server/scripts`, and `tests` | this matrix plus `tools/parity`, `tools/scriptinventory` | every numbered owner is listed here | matrix complete; behavior open |
| 3 | `src/server/game/Server/WorldSocket.cpp`, `WorldSession.cpp`, and packet handlers | `docs/NEXT4_PACKET_STATE_REPLAY.md`, `pkg/protocoltrace`, `tools/packettrace`, `engine/world/server.go`, `engine/auth/trace.go`, `engine/service/service.go`, `engine/database/database.go` | source-defined JSON packet replay, failure replay, and redacted database transition results pass | complete for source-defined replay; client/login state evidence remains in items 4–5 |
| 4 | `Handlers/CharacterHandler.cpp`, `Entities/Player/Player.cpp::SendInitialPacketsBeforeAddToMap/AfterAddToMap`, `Maps/Map.cpp::SendInitSelf/AddPlayerToMap`, `Maps/Map.cpp::GetZoneAndAreaId` | `engine/world/characters.go`, `worldstates.go`, `terrain.go`, `player_packets.go`, `player_state.go`, `transports.go`, `death.go`, `skills.go`, `guild.go`, `dynamic_spells.go`, `tools/logincheck`, `tools/terraincheck`, `tools/worldstatecheck` | source-defined login order, single visibility pass (`879320e`), deterministic talent order (`f505235`), attached/map transport create phases (`35f3d8d`), attached transport passenger create batch (`5a30b16`), player GUID low-word update-mask correction and source-order follow-up (`901c330`), `PLAYER_CHOSEN_TITLE` field correction (`3307faa`), public player update-field flag correction (`939d6fb`), create-block field verifier (`c8dbd96`), mandatory achievement/equipment login-order stages (`1b0473a`), map 530/zone 3430 world-state fixture (`708486c`), hidden-duration aura packet correction (`a0a25e6`), live-aura hidden-duration correction (`0a9d5e9`), persisted pet-aura hidden-duration correction (`49f74ab`), aura effect-mask correction (`19dedcb`), runtime aura effect-index correction (`080f4e6`), single aura-update effect-index correction (`bb64c14`), packed achievement timestamps (`5dad1ad`), post-load achievement completion timing (`cb7dfeb`), hidden achievement visibility (`fc28f25`), creature cast-speed/hover-height fields (`bca9cf4`), creature query fallback correction (`1c4343c`), transport dynamic path progress (`10ca358`), transport route period (`2e078a9`), transport passenger hover height (`bc57f4d`), supplied September login trace correlation (pre-verify `SMSG_CRITERIA_UPDATE`), DBC reputation-list mapping (`286d282`), active pet create fields (`79e5df9`), pet ActiveStates preservation (`47cd644`), guild roster state (`0c74b76`), source login time speed (`a47bf9d`), regression verifier for pre-verify achievement packets (`b1deed4`), inventory create-field boundary (`e9e239b`), packed login time (`8506995`), ready rune resync (`118cd98`), triggered login effect (`c69879c`), cinematic branch (`50caa6b`), time-sync schedule (`a3ccee0`), guild bank login packet (`ba58ec7`), guild signed-on event (`8fe0b3b`), guild MOTD event (`6a88a62`), saved player appearance fields and source-shaped empty guild-event parameters (`80ed6b9`), nearby-player public visibility (`278c4bb`), reciprocal player create broadcast (`351037c`), stateful player visibility/removals (`0c2de4e`), dynamic spell object login visibility (`a7206c4`), nearby corpse/bones visibility (`2397643`), recorded login order/create-block verifier (`ef02ca3`), selected-title compatibility guard (`225064a`), declined-name query payload (`0f87368`), group HighGuid encoding (`a3e0100`), group `SMSG_GROUP_LIST` GUID/layout validator (`3d10e2d`), persisted LFG dungeon/state login fields (`3ebf379`), separate runtime/database group GUID loading (`8f3f574`), source member-order preservation (`a21c0d8`), character customization-flag precedence (`67ca009`), login quest-sharing details replay (`6025328`), difficulty, cooldown, unlearn, movement, corpse, terrain/WMO area, and world-state fixtures | closed: source packet sequence, create-block/update-mask validation, and post-map packet ordering are enforced by logincheck; compatible-client transport/gameplay acceptance remains under its own source rows |
| 5 | `Entities/Player/Player.cpp::LoadFromDB`, `_LoadSkills`, `_LoadSpells`, `_LoadInventory`, `_LoadAuras`, `_LoadQuestStatus` | `engine/world/player_state.go`, `player_packets.go`, `player_auras.go`, `characters.go`, `items.go`, `quests.go`, `skills.go`, `pets.go` | focused equipment, spell, language, glyph/aura, rested, real-time item-duration, cinematic, pet-cooldown, reset tests; inventory create-field boundary (`e9e239b`); persisted pet summon-effect replay (`4ea1f9b`); summoned-pet owner-level normalization (`a938e7e`); failed/non-`NONE` quest status restoration (`a9c7ac9`); rewarded quest title, bonus-talent, and learn-spell restoration (`af6c6cf`, `6f99971`, `7717bef`); database-backed default skill reconstruction (`fab0fb1`); inventory-derived visible equipment cache (`ea4e3b0`); arena-team create fields (`d3054ff`); random-battleground winner status (`2aa52e6`); persisted battleground join/team/taxi/mount state (`04d4124`); battleground reconnect status reconstruction (`fe73b67`); weekly/monthly/seasonal quest cooldown gates (`d1bac35`); bound-instance validation (`887639b`); account instance lock-time loading (`d5863a2`); periodic quest status containers (`2fa77bc`); expired-mail login cleanup (`fa37504`); invalid inventory item cleanup (`9761ff2`); offline conjured/refund item cleanup (`fc07e19`); invalid bag-parent, duplicate-position, and out-of-range bag-slot cleanup before item serialization (`8204a10`); BOP-tradeable flag/data validation for missing metadata, invalid looter sets, stackability, and soulbound state (`1445884`); inactive holiday-item removal with scheduled holiday-ID evaluation (`6c98525`); creator/gift-creator GUID load and item update-field serialization (`e22c056`); duration, no-bind soulbound, and durability load normalization with persisted corrections (`c3a1513`); negative random-property suffix-factor reconstruction from `RandPropPoints.dbc` (`30ff94b`); persisted talent-reset cost/time restoration (`7bf426e`); offline drunkenness decay (`e9f8f3f`); persisted money clamp to `MAX_MONEY_AMOUNT` (`3e35c1a`); full 64-bit known-currency field restoration (`7529f1d`); forced-rename login rejection (`909e4a5`); taxi-mask initialization and partial saved-mask application (`eb9de63`); DK old-continent and level-dependent taxi initialization (`908bbe3`); saved taxi-mask filtering against the DBC valid-node mask (`1c32c25`); primary-profession field restoration and configurable cap (`f407b3d`); skill-line rewarded spell reconstruction (`a3e3a2f`); configured max level-scaled skill values (`0ee6135`); DBC race/class skill legality (`16726dd`); DBC skill range classification (`1343fbe`); DBC skill-step serialization (`689c224`); saved spell race/class legality (`52a8206`); complete player aura persisted state (`55d6019`) | open: complete character-state diff |
| 6 | `Handlers/LogoutHandler.cpp`, `Entities/Player/Player.cpp::SaveToDB`, `WorldSession` teardown | `engine/world/characters.go`, `server.go`, `player_state.go` | logout and persistence tests | open: full normal logout packet/database fixture |
| 7 | logout combat, taxi, battleground, disconnect, and character-switch branches in `LogoutHandler.cpp`, `WorldSession.cpp` | `engine/world/characters.go`, `server.go`, `battleground.go`, `taxi.go` | targeted lifecycle tests | open: complete edge-case matrix |
| 8 | `auth/AuthSocket.cpp`, `Handlers/AuthSession.cpp`, `WorldSocket.cpp` | `engine/auth`, `engine/crypto`, `server/authserver` | SRP, reconnect, version, ban, and duplicate-session tests | open: result/disconnect parity matrix |
| 9 | `Spells/Spell.cpp`, `SpellEffects.cpp`, `Spells/Auras/*`, `Unit.cpp` | `engine/world/spells.go`, `spell_stats.go`, `dynamic_spells.go`, `player_auras.go` | cast, GCD, interrupt, resistance, aura, and target tests | open: DBC-backed resource/aura fixtures |
| 10 | `Entities/Creature/Creature.cpp`, `Entities/Unit/Unit.cpp`, `Combat/ThreatManager.cpp`, AI owners | `engine/world/combat.go`, `creaturemotion.go`, `threat.go`, `kill.go`, `pet_combat.go` | deterministic threat, combat, evade, and death tests | open: full target-switch and persistence parity |
| 11 | `Movement/*`, `Entities/Unit/Unit.cpp`, `Maps/Transport.cpp`, `TransportMgr.cpp` | `engine/world/movement.go`, `transports.go`, `position.go` | source-defined transport-relative GUID/offset, taxi, attached/map transport login creates (`35f3d8d`), live player passenger creates (`bfbfbba`), cross-map transfer-pending/new-world packets, and post-worldport map-entry phases (`d63dfac`) | open: live boat/zeppelin spline and client-compatibility verification |
| 12 | `Handlers/LootHandler.cpp`, `Loot/Loot.cpp`, corpse and map ownership paths | `engine/world/loot.go`, `kill.go`, `death.go` | loot, money, group, corpse, and release tests; nearby corpse visibility (`2397643`) | open: complete eligibility/persistence replay |
| 13 | `Handlers/NPCHandler.cpp`, `Entities/Item/Item.cpp`, vendor/buyback state | `engine/world/vendors.go`, `items.go`, `characters.go` | vendor and buyback fixtures | open: unlimited/limited stock and relog parity |
| 14 | `Handlers/GossipHandler.cpp`, `NPCHandler.cpp`, `GameObject.cpp`, spirit-healer and transport handlers | `engine/world/gossip.go`, `npc_interaction.go`, `gameobjects.go`, `death.go`, `transports.go` | gossip, NPC, gameobject, spirit, and reduced-schema tests | open: valid/rejected/ghost/GM client stability |
| 15 | `Handlers/ChatHandler.cpp`, `Chat/Channel.cpp`, `Chat/Chat.cpp` | `engine/world/chat.go`, `channels.go`, `social.go` | source-defined chat/channel and encrypted chat fixtures; guild/officer speak/listen rights now use the pinned `GR_RIGHT_*CHAT*` bit values (`63f675b`); client-sent universal language is rejected before permission-based server language rewriting (`e14c976`); addon chat is gated by the source `AddonChannel` setting (`0d53d23`), bypasses normal-text control-character rejection (`62b6183`), does not route through normal commands (`cf35c4e`), omits `WHISPER_INFORM` for addon whispers (`e7dc99d`), canonicalizes city-only built-in channel identities (`839de9e`), silently rejects out-of-area system-channel joins (`1970684`), scopes channel namespaces by faction team (`1579447`), and rejects hyperlink channel names (`8139b02`) | open: all channel/language/addon routing verification |
| 16 | `Groups/*`, `Guilds/*`, `Trade/*`, `Mail/*`, `AuctionHouse/*`, calendar and equipment-set owners | `engine/world/group.go`, `guild.go`, `trade.go`, `mail.go`, `auctionhouse.go`, `calendar.go`, `items.go` | focused state-machine tests by subsystem; source-order guild login bank packet (`ba58ec7`), signed-on event (`8fe0b3b`), MOTD event (`6a88a62`), roster bank rights (`57f4140`), corrected group login packet state (`c1af4ac`), raw `HighGuid::Group` login GUID encoding (`a3e0100`), group `SMSG_GROUP_LIST` GUID/layout validator (`3d10e2d`), persisted `lfg_data` dungeon/state login fields (`3ebf379`), completed-dungeon LFG state transition and persistence (`121d965`), battleground-only member PvP status (`e988e2c`), separate runtime/database group GUID loading (`8f3f574`), source member-order preservation (`a21c0d8`), persisted difficulty clamping (`84003a6`) | open: full normal/failure persistence matrix |
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

Item 4's source-defined login packet-order and update-mask gate is closed; item 5's persisted character-state comparison remains active, and later rows are not promoted by inventory counts.

The login regression verifier includes an executable self-check (`go run ./tools/logincheck -self-check`) for the pre-`SMSG_LOGIN_VERIFY_WORLD` achievement-packet ordering rule (`a8c2896`).

The same verifier now decodes compressed updates, walks merged item/player blocks, and validates the self-player create movement framing, 42-field-block mask, required fields, and range bits with deterministic compressed/uncompressed fixtures (`39e265d`).

Login replay now rejects cinematic packets before world verification or after the self-player create block, matching the source first-login cinematic branch (`7909904`).

Achievement criteria state is now retained/persisted during loading without emitting `SMSG_CRITERIA_UPDATE` until `PlayerLoading` clears, matching the source login suppression path (`a552726`).

Login movement replay now rejects movement-state packets before `SMSG_TIME_SYNC_REQ`, matching `Player::SendInitialPacketsAfterAddToMap` (`6868aa8`).

The replay self-check now proves both rejection of pre-verify achievement packets and acceptance of criteria/earned packets after `SMSG_LOGIN_VERIFY_WORLD` (`0eaba87`).

Login replay now validates the `SMSG_LOGIN_SET_TIME_SPEED` packed-time, fixed `0.5` speed, zero holiday offset, and exact payload length (`fe05780`).

Login replay now validates payload layouts and exact lengths for world verification, instance difficulty, initial/unlearned spells, action buttons, and the 128-entry faction table (`c5a3d78`).

Login replay now validates fixed lengths for dance moves, feature status, and bind-point packets against `MiscPackets.cpp` (`47bdb7d`).

The login fixture suite now exercises exact payload contracts for world verification, instance difficulty, initial and unlearned spells, action buttons, faction initialization, dance moves, feature status, bind point, and login time speed (`774de93`).

The login effect `SMSG_SPELL_GO` now matches `Spell::SendSpellGo`: the self packet carries `CAST_FLAG_POWER_LEFT_SELF` and remaining power, while the nearby packet removes both fields; the fixture validates the complete payload (`c396319`).

Login `SMSG_INSTANCE_DIFFICULTY` now derives its value from the current map type and instance difficulty, keeping saved dungeon settings off continent maps and matching `Map::GetDifficulty` (`cae1d75`).

The shared player cast path now sends `CAST_FLAG_POWER_LEFT_SELF` with remaining power to the caster and a flag-cleared `SMSG_SPELL_GO` to nearby clients, matching `Spell::SendSpellGo` (`a7871db`).

The login replay verifier now validates `SMSG_INIT_WORLD_STATES`, `SMSG_SET_FORCED_REACTIONS`, six-rune `SMSG_RESYNC_RUNES`, and the initial zero-counter `SMSG_TIME_SYNC_REQ` payload contracts (`4fd9463`).

The same verifier now validates aura replay records plus the fixed 12-byte item-duration and 24-byte enchant-duration payloads delivered after map entry (`7c99054`).

Recorded login traces now receive the same optional-packet validation for rune resync, aura replay, item durations, enchant durations, and quest-giver status entries, preventing malformed post-map packets from passing replay (`40738f7`).

The replay gate now also validates permanent-pet login `SMSG_PET_SPELLS` framing, including the 10-button action bar, spell list, and cooldown records used by active summoned pets (`5a8ad4a`).

Recorded login traces now validate taxi-node status records and quest-giver status-multiple count/GUID/status framing as well (`16a408c`).

The create-update verifier now parses the complete self-login update batch and rejects item/container creates after the player create or duplicate self-player blocks, matching `Player::BuildCreateUpdateBlockForPlayer` ordering (`2fd2be3`).

Attached transport login and worldport batches now include live player passengers with transport-relative movement creates and mark them visible to prevent duplicate nearby-player creates, matching `Map::SendInitSelf` passenger handling (`bfbfbba`).

The post-online group packet now preserves TrinityCore group type/LFG fields, per-recipient counters, member online/PvP status, master-looter zeroing, and dynamic raid difficulty (`c1af4ac`).

Corpse repopulation now occurs after the loading flag clears and ON_LOGIN criteria update, immediately before the login hook, matching `CharacterHandler.cpp` (`e083f6a`).

The self-player update now serializes the computed shield-block value into `PLAYER_SHIELD_BLOCK`, matching `Player::UpdateAllStats` field delivery (`43c5918`).

Self-player update fields now separate total stats from positive equipment/stat bonuses, matching `Unit::BuildValuesUpdate` and `Player::UpdateAllStats` (`746eb67`).

Player melee, ranged, offhand, and school spell crit fields now derive from the pinned GT chance/rating DBC tables instead of fixed constants (`f0f2ae3`).

Player block and dodge percentages now use the pinned GT rating/chance data and defense-skill contribution during self-player field construction (`050265b`).

Self-player expertise and shield-block fields now use the source rating coefficient and strength-derived block value (`f072044`).

Self-player client-side spell-power state now serializes healing and per-school positive damage fields from the loaded spell-power total (`cb9eebd`).

Self-player spell penetration now serializes the source negative target-resistance field from equipped-item state (`22b34c6`).

Self-player positive/negative per-school damage fields now include loaded damage-modifying auras in addition to item spell power, matching `UpdateSpellDamageAndHealingBonus` (`f1bab31`).

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

Login quest reconstruction now retains every non-`NONE` quest status, restores the source failed-state bit, and normalizes invalid statuses to incomplete, matching `Player::_LoadQuestStatus` (`a9c7ac9`).

Login now clears a persisted chosen title absent from the known-title mask, matching `Player::LoadFromDB`'s `PLAYER_CHOSEN_TITLE` compatibility check (`225064a`).

Connected-player name queries now append persisted five-case declined names, matching `QueryHandler::SendNameQueryOpcode` (`0f87368`).

Login now reapplies rewarded quest title masks and the source Ebon Hold bonus-talent counter, matching `_LoadQuestStatusRewarded` and `CalculateTalentsPoints` (`af6c6cf`, `6f99971`).

Rewarded quest `RewardSpell` learn effects now restore their triggered spells before initial-spell serialization, matching `Player::LearnQuestRewardedSpells` (`7717bef`).

Missing default defense, unarmed, weapon, armor, class, and language skills are now reconstructed from `playercreateinfo_skills` during login, matching `LearnDefaultSkills`/`UpdateSkillsForLevel` behavior (`fab0fb1`).

Character enumeration and login now rebuild visible equipment from current inventory rows instead of trusting a stale nonzero `equipmentCache`, matching the source inventory load's visible-item fields (`ea4e3b0`).

Login now restores arena-team IDs, brackets, captain/member state, weekly/season games and wins, and personal ratings into the private player update fields, matching `Player::_LoadArenaTeamInfo` (`d3054ff`).

Login now restores the random-battleground winner flag from `character_battleground_random`, matching `Player::_LoadRandomBGStatus` and exposing it in battlefield-list status (`2aa52e6`).

Login now loads `character_battleground_data` instance, team, join location, taxi nodes, and mount spell fields into the session, matching `Player::_LoadBGData` (`04d4124`).

When persisted battleground data points to a battleground map, login now reconstructs an in-progress battlefield status slot using the saved instance/team, matching the reconnect-visible portion of `Player::_LoadBGData` (`fe73b67`).

Quest acceptance now rejects persisted weekly, monthly, and seasonal cooldown rows using the source special flags and event relation, matching `SatisfyQuestWeek`, `SatisfyQuestMonth`, and `SatisfyQuestSeasonal` (`d1bac35`).

Login now removes invalid or contradictory character instance binds before instance difficulty state is used, matching `_LoadBoundInstances` validation (`887639b`).

Login now loads active `account_instance_times` release timestamps and excludes expired restrictions from the session state, matching `_LoadInstanceTimeRestrictions` (`d5863a2`).

Login now loads weekly, monthly, and seasonal quest cooldown containers into player state, matching `_LoadWeeklyQuestStatus`, `_LoadMonthlyQuestStatus`, and `_LoadSeasonalQuestStatus` (`2fa77bc`).

Login now runs expired-mail return/deletion cleanup before unread-mail state and `SMSG_RECEIVED_MAIL`, matching the source mail lifecycle (`fa37504`).

Login now deletes inventory rows whose item instances are missing templates, nonpositive, or invalid before client create serialization, matching `_LoadItem` validation (`9761ff2`).

Offline login now removes expired conjured items and clears stale or expired refundable-item flags/metadata, matching `_LoadItem`'s time and refund branches (`fc07e19`).

Login inventory reconstruction now rejects duplicate positions, duplicate item references, orphaned bag GUIDs, non-container parents, and child slots outside the source bag capacity before create updates are serialized, matching `_LoadInventory`'s bag map and invalid-position handling (`8204a10`).

Login now validates `ITEM_FIELD_FLAG_BOP_TRADEABLE` against `item_soulbound_trade_data`, the distinct looter set, single-stack template state, and the soulbound item flag, clearing the stale flag and metadata under the same conditions as `Item::ClearSoulboundTradeable` (`1445884`).

Login now evaluates active `game_event.holiday` IDs and removes inventory items whose `item_template.HolidayId` is not active, matching `_LoadItem`'s holiday branch and preserving the source event schedule rules (`6c98525`).

Login item create updates now carry persisted creator and gift-creator player GUID pairs from `item_instance`, matching `Item::LoadFromDB` and the public `ITEM_FIELD_CREATOR`/`ITEM_FIELD_GIFTCREATOR` fields (`e22c056`).

Login now applies `Item::LoadFromDB` corrections for template duration presence, `NO_BIND` soulbound flags, and non-wrapped durability above `MaxDurability`, persisting the corrected duration/flags/durability tuple before serialization (`c3a1513`).

Negative random-property items now select the source suffix coefficient from `RandPropPoints.dbc` using item level, quality, and inventory type, then serialize `ITEM_FIELD_PROPERTY_SEED` (`30ff94b`).

Login now restores `resettalents_cost` and `resettalents_time` before talent-reset calculations, matching `Player::LoadFromDB` (`7bf426e`).

Login now reduces saved drunkenness by elapsed logout time using the source nine-seconds-per-value rule, matching `Player::LoadFromDB` (`e9f8f3f`).

Login now clamps persisted money to the source `MAX_MONEY_AMOUNT` (`INT32_MAX`) before player fields and later saves are produced (`3e35c1a`).

Login now restores both words of the source 64-bit `PLAYER_FIELD_KNOWN_CURRENCIES` update field instead of truncating it to one `uint32` (`7529f1d`).

Login now rejects characters carrying `AT_LOGIN_RENAME`, matching the source `Player::LoadFromDB` forced-rename gate (`909e4a5`).

Login now seeds race/team taxi nodes before applying the persisted taxi mask and preserves initialized words when the saved mask is partial or empty, matching `PlayerTaxi::InitTaxiNodesForLevel` and `LoadTaxiMask` (`eb9de63`).

Taxi initialization now also restores the Death Knight old-continent mask and the level-68 Shattered Sun node 213 branch from `PlayerTaxi::InitTaxiNodesForLevel` (`908bbe3`).

Saved taxi-mask words are now intersected with the DBC-derived valid taxi-node mask, matching `PlayerTaxi::LoadTaxiMask` (`1c32c25`).

`PLAYER_CHARACTER_POINTS2` now carries the configurable free primary-profession cap (default 2) instead of the spent-talent count, matching `InitPrimaryProfessions` (`f407b3d`).

Pet aura loading now preserves the complete persisted effect and resilience metadata with the source reduced-schema fallback (`44ff067`).

Login now reconstructs SkillLineAbility spells from loaded skill values with source acquire-method, race/class, threshold, and supersession rules before initial-spell serialization (`a3e3a2f`).

Level-scaled skills now apply the configured `AlwaysMaxSkillForLevel` behavior during login, matching `Player::UpdateSkillsForLevel` (`0ee6135`).

Persisted and starter skill rows now use `SkillRaceClassInfo.dbc` race/class masks when available, matching `_LoadSkills` legality checks (`16726dd`).

Skill normalization now uses DBC `SkillLine` categories, `SkillTiers`, and runeforging/armor special cases to classify language, mono, level, and rank ranges (`1343fbe`).

Login skill update fields now derive the source skill step from `SkillTiers.dbc` instead of hardcoding step 1 (`689c224`).

Saved spell rows now pass SkillLineAbility race/class masks and SkillRaceClassInfo legality before being exposed as active spells (`52a8206`).

Player aura loading now preserves item/caster GUIDs, recalculate masks, all effect/base amounts, crit chance, and resilience metadata, with a reduced-schema fallback (`55d6019`).

Persisted achievement loading now rejects unknown Achievement.dbc and criteria rows and drops expired timed criteria before `SMSG_ALL_ACHIEVEMENT_DATA`, matching `AchievementMgr::LoadFromDB` (`e11614d`).

Persisted action buttons now reject unknown spells, missing item templates, and unsupported action types before `SMSG_ACTION_BUTTONS`, matching `Player::IsActionButtonDataValid` (`b7741fd`).

Persisted character-spell rows now preserve the source active/disabled state and only reject unknown DBC spells; class/race and level gates remain in `LearnSkillRewardedSpells`, not `_LoadSpells` (`22eb90b`).

Persisted glyphs now require matching `GlyphProperties.dbc` and `GlyphSlot.dbc` slot types before their aura is applied, matching `_LoadGlyphAuras` (`14c27bc`).

Faction login state now keeps the persisted standing offset separate from DBC base standing, matching `ReputationMgr::Initialize` and `LoadFromDB` (`426e6fd`).

Login and runtime action-bar toggles now serialize `PLAYER_FIELD_BYTES` byte 2, matching `HandleSetActionBarToggles` and `Player::LoadFromDB` (`31785e3`).

Offline rest-bonus loading now honors the configured max-level rule and serializes `PLAYER_REST_STATE_EXPERIENCE`, matching `SetRestBonus` (`c2ed09d`).

Character login now rejects active character bans, invalid names, invalid gender values, and invalid race/class definitions before state or packet construction, matching `Player::LoadFromDB` (`d00b9da`).

Persisted player positions and map IDs now recover to the homebind before login serialization when invalid, matching the source map/coordinate recovery path (`2e8c0d8`).

GM login now restores `GM.VisibleState` and saved invisible state with the source GM-on/visibility semantics, matching `Player::SetGMVisible` (`63d5205`).

Runtime `.gm visible` transitions now restore GM mode and refresh/destroy nearby visibility state, matching `SetGMVisible` and `UpdateObjectVisibility` (`e5dcf0c`).

Character login now captures the persisted first-login flag and fires Eluna `PLAYER_EVENT_ON_FIRST_LOGIN` (event 30) before the normal login event, matching `ScriptMgr::OnPlayerLogin` (`435f5b9`).

The login replay verifier now parses `SMSG_QUEST_GIVER_QUEST_DETAILS` counts, reward fields, faction arrays, and emote framing for quest-sharing replay (`b077406`).

Persisted aura loading now rejects zero and out-of-range effect masks before aura construction, matching `_LoadAuras` and `Aura::TryCreate` input requirements (`8e97006`).

Persisted aura loading now skips passive and channeled spells, matching `Aura::CanBeSaved` (`88c5e78`).

Persisted aura loading now rejects source-unsavable control/vehicle/bind-sight effects, item-permanent auras, and the pinned temporary aura blacklist (`ab58402`).

`ChrRaces.dbc` resurrection-sickness spell IDs are now loaded and used for offline aura decay instead of a fixed spell ID (`1cbd245`).

Persisted groups now allocate runtime `HighGuid::Group` lows deterministically from valid database-group order instead of first-login order (`1dfdeca`).

Persisted aura loading now honors TrinityCore's `MAX_AURAS` capacity of 255 instead of truncating at 64 (`70fa46c`).

Login inventory create blocks now follow TrinityCore equipment/top-level slot order and recursively emit bag contents by slot (`2dcc610`).

Login questgiver status now includes visible gameobject questgivers, source-style three-dimensional range filtering, and raw creature/gameobject GUIDs (`6597c3d`).

Taxi status packets now report only the persisted taxi-mask bit, matching `Player::SendTaxiStatus` and `SendTaxiNodeStatusMultiple`; taxi-cheat authorization remains separate (`f01ce06`).

Equipment-set login entries now encode saved piece counters as Item high GUIDs before `SMSG_EQUIPMENT_SET_LIST`, matching `_LoadEquipmentSets` (`59a7417`).

Login now emits the zero-count equipment-set list packet when the set table is empty, unavailable, or reduced, preserving TrinityCore's packet sequence (`fe2b826`).

Saved equipment sets with client-invalid indexes are now rejected during login loading at the pinned ten-slot limit (`eeba440`).

Login flightmaster status candidates now use the source visible-object three-dimensional range (`55eb76a`).

The login replay harness now parses and validates `SMSG_EQUIPMENT_SET_LIST` framing, packed item GUIDs, client set limits, and trailing bytes (`66e7412`).

The login replay harness now enforces source post-map ordering across world states, time sync, login effect, aura/duration updates, quest/taxi statuses, raid difficulty, quest sharing, group state, and pet state (`931317f`).

Login raid-difficulty comparison now stores the current map difficulty, including zero outside raids, before deciding whether to emit `MSG_SET_RAID_DIFFICULTY` (`8c208e0`).

Login corpse restoration now clears stale resurrectable corpse rows for characters with saved health instead of incorrectly setting ghost/repop state (`3658fcd`).

Contact and guild roster loading now preserve source map ordering, and the login replay harness validates contact-list, guild-event, guild-bank, and guild-roster framing (`069a666`).

Cinematic opening, next-camera, and completion handlers now leave cinematic persistence to the login/save owners, matching `CinematicMgr` packet-handler ownership (`9e11e7d`).

Login now suppresses persisted corpse-object creates when the source state is scheduled for `BuildPlayerRepop`, preventing an old corpse from being sent before the new ghost corpse is created (login-corpse-repop milestone).

GameDataDir resolution now falls back to the configured DataDir, allowing MPQ-derived `dbc`, `maps`, `vmaps`, and `mmaps` directories directly under the configured bin layout (runtime-input path milestone).

World initialization now reports the exact required login DBC files when the configured game-data directory is incomplete, preventing silent DBC-backed client-state failures (runtime-input diagnostics milestone).

Saved-pet restoration emits the source fake-summon `SMSG_SPELL_GO` with `CAST_FLAG_UNKNOWN_9` for `CreatedBySpell`, matching `Pet::LoadPetFromDB` packet ownership (saved-pet login milestone).

Group-list counters are now allocated atomically per emitted packet, matching TrinityCore's `m_counter++` semantics without a read-lock data race (group-login counter milestone).

Saved pet action-bar serialization now preserves TrinityCore default command/spell/reaction slots when custom action-bar tokens are malformed, while retaining valid custom slots (saved-pet action-bar milestone).

Saved pet spell packets now discard invalid DBC-known spell rows before `PetSpellInitialize` serialization, while retaining rows when DBC input is unavailable (saved-pet spell validation milestone).

Saved pet action-bar entries now clear unknown DBC-known spells and force non-autocastable spells to passive, matching `CharmInfo::LoadPetActionBar` and the pinned `SPELL_ATTR0_PASSIVE`/`SPELL_ATTR1_UNAUTOCASTABLE_BY_PET` rules (saved-pet action-bar validation milestone).

Saved pets now use model combat reach plus `PET_FOLLOW_DIST` and `PET_FOLLOW_ANGLE` for the initial close-point position, and serialize the same model bounding radius and combat reach into the create update when model data is available, matching `Pet::LoadPetFromDB` and `WorldObject::GetClosePoint` (saved-pet geometry milestone).

Saved summon pets now serialize the source Mage class and mana power fields for every `SUMMON_PET`, while hunter pets retain Warrior class and focus power, matching the `Pet::LoadPetFromDB` pet-type branches (saved-pet class/power milestone).

Active saved pets with zero persisted health now still reach create, aura, and pet-spell restoration, matching `Player::LoadPet` and `Pet::LoadPetFromDB` dead-hunter handling instead of being silently omitted at login (saved-pet death-state milestone).

The login replay checker now validates the pre-map dungeon-difficulty flags, `PER_CHARACTER_CACHE_MASK` account-data-times framing, and MOTD line framing emitted by `CharacterHandler.cpp` before accepting the player-create boundary (pre-map login payload milestone).

Contact-list and friend-status serialization now preserve TrinityCore's bit-valued AFK/DND/RAF states, emit status fields for online-result packets even when the target has gone offline, and include zone/level/class only for non-offline list entries, matching `PlayerSocial::SendSocialList`, `SocialMgr::GetFriendInfo`, and `SocialMgr::SendFriendStatus` (social login packet milestone).

Runtime faction hostility, vendor reputation requirements, and reputation achievements now evaluate DBC base standing plus the persisted character-reputation offset, matching `ReputationMgr::GetReputation` and `LoadFromDB` instead of treating the stored offset as an absolute standing (reputation-state milestone).

The login replay checker now parses `SMSG_TALENTS_INFO` player and pet variants, including spec counts, active spec, learned talent records, and glyph slots, matching `BuildPlayerTalentsInfoData` and `BuildPetTalentsInfoData` (talent login packet milestone).

The login replay checker now parses both terminated sections of `SMSG_ALL_ACHIEVEMENT_DATA`, including packed criterion counters/player GUIDs, flags, and timestamp fields, matching `AchievementMgr::BuildAllDataPacket` (achievement login packet milestone).

Optional login revision chat now follows `CONFIG_ENABLE_SINFO_LOGIN` through `Server.LoginInfo` and `MORENOCORE_SERVER_LOGIN_INFO`, sending the Go build revision immediately after MOTD like `CharacterHandler.cpp` (login server-info milestone).

`PlayerStart.AllReputation` now emits the source `SMSG_SET_FACTION_STANDING` transition after initial faction initialization, preserves visible faction flags, and reports rank increases like `ReputationMgr::SetOneFactionReputation` and `SendState` instead of sending a second initialization packet (start-reputation login milestone).

Dead player login now keeps saved health at zero through the self-player create update and explicitly masks that zero field, leaving `BuildPlayerRepop` to set health to one at the source post-login repop boundary (dead-player update-mask milestone).

Login now starts an in-game clock after account-online state, advances total/level played time on saves and played-time queries, and persists the counters, matching `Player::SetInGameTime`, `Player::Update`, and `HandlePlayedTime` (played-time login-state milestone).

Name-query handling now preserves the requested packed GUID while resolving player cache/database and declined-name records through the source `HighGuid::Player` low counter, matching `WorldSession::HandleNameQueryOpcode` and `SendNameQueryOpcode` (player-name login visibility milestone).

Initial spell cooldown loading now retains rows with an expired spell timer but an active category timer, matching `SpellHistory::WritePacket<Player>` and the source cooldown retention boundary (login cooldown packet milestone).

The login replay checker now exercises that category-only cooldown case with a one-spell `SMSG_INITIAL_SPELLS` fixture requiring zero spell duration and a positive category duration (login cooldown packet evidence milestone).

The login replay checker now validates known and unknown `SMSG_NAME_QUERY_RESPONSE` forms, including packed GUID, character fields, and declined-name framing (player-name packet evidence milestone).

The login replay checker now validates the nine-byte `SMSG_PLAYED_TIME` total/level/trigger response framing (played-time packet evidence milestone).

The self-player create fixture now requires populated visible equipment entry/enchantment fields at `PLAYER_VISIBLE_ITEM_1_ENTRYID`, matching the source public update-field boundary used by `Player::BuildCreateUpdateBlockForPlayer` (equipment visibility packet evidence milestone).

Mounted login reconstruction now sets `UNIT_FLAG_MOUNT` whenever a persisted mounted aura supplies `UNIT_FIELD_MOUNTDISPLAYID`, matching `Unit::Mount` and preventing a display-only mount create update (mounted login field milestone).

The login replay checker now validates the `SMSG_SET_FACTION_STANDING` float/flag/count/standing-entry framing used by the start-reputation transition (start-reputation packet evidence milestone).

Character name lookup now retains the complete character-enum cache before world entry and resolves packed player GUIDs through the online session, enum cache, and low-counter database paths without closing the connection on a lookup error, matching `QueryHandler.cpp` and the global `HighGuid::Player` counter (character-name visibility correction).

Continent boats and zeppelins now serialize the source mobile-transport GUID high value `0x1FC0`, persist only the transport low counter in `characters.transguid`, normalize client transport attachments, and retain compatibility reads for the previous game-object GUID form, matching `TransportMgr::CreateTransport`, `Transport::AddPassenger`, and `MovementHandler.cpp` (continent-transport GUID correction). Runtime initialization loaded 20 configured continent transports in the current local world database; client boarding and cross-map world-port replay remain open acceptance evidence.

Creature activation now honors persisted `creature.curhealth` instead of manufacturing health for zero-health rows, and first activation clamps current health to source-calculated template health, matching `Creature::SetSpawnHealth` and preventing dead/low-health critters from becoming inflated combat targets (creature current-health correction).

Runtime mounted auras now preserve source infinite `SpellDuration` values, remove competing mounted auras, resolve the mount creature display, set and clear `UNIT_FLAG_MOUNT` with `UNIT_FIELD_MOUNTDISPLAYID`, broadcast the update, and remove the aura on cancel, matching `AuraEffect::HandleAuraMounted` and `Unit::Mount` (runtime mount-state correction).

First-login cast spells now use the triggered-cast path, avoiding normal player power consumption, cooldown persistence, and cast-start state while preserving the source `Player::CastSpell(..., true)` `SMSG_SPELL_GO` effect application (first-login triggered-spell correction).

The login replay checker now validates a non-login-effect triggered first-login `SMSG_SPELL_GO` with `CAST_FLAG_UNKNOWN_9`, pending, self-power, self-target, and remaining-power framing, matching `Spell::SendSpellGo` for `Player::CastSpell(..., true)` (first-login triggered-spell packet evidence).

The replay gate now enforces the optional pre-map guild login window: guild MOTD, bank-list, and roster packets must occur after MOTD and before learned dance moves in the source order from `Guild::SendLoginInfo` and `CharacterHandler.cpp` (pre-map guild login ordering evidence).

Persisted and runtime stun/root auras now retain server movement-control state, set the source stunned flag, replay force-root state, reject movement while rooted, and clear state with force-unroot when the final control aura is removed, matching `HandleAuraModStun`, `HandleAuraModRoot`, and `Player::SendInitialPacketsAfterAddToMap` (login movement-control correction).

Persisted stun auras now also set `UNIT_FLAG_STUNNED` before the self-player create update is serialized, matching the source aura application state visible in `Player::BuildCreateUpdateBlockForPlayer` (persisted control-flag update-mask correction).

Persisted and runtime fake-inebriation aura amounts now populate and maintain the public `PLAYER_FAKE_INEBRIATION` field separately from raw drunkenness, matching `AuraEffect::HandleAuraModFakeInebriation` and `Player::SetDrunkValue` (fake-inebriation update-field correction).

Initial nearby-player visibility and reciprocal player-create broadcasts now apply GM-invisibility, ghost, faction, and group visibility checks, matching `WorldObject::CanSeeOrDetect` and `Player::UpdateVisibilityForPlayer` (player visibility correction).

The same login visibility paths now apply the pinned stealth detection calculation, including GM/group/track-stealth/contact/arc/rating behavior from `WorldObject::CanDetectStealthOf` (stealth visibility correction).

Player visibility now also compares each target invisibility aura type/value against the observer's matching detection auras, with GM override, matching `WorldObject::CanDetectInvisibilityOf` (invisibility visibility correction).

Live player value updates now build separate self and nearby-client masks, filtering the nearby packet through the source `UF_FLAG_PUBLIC` field set instead of rebroadcasting owner/private fields. The regeneration path uses the same split, and `tools/logincheck --self-check` validates public health/display fields while rejecting a private talent field, matching `Object::_BuildValuesUpdate` and `UnitUpdateFieldFlags.cpp` (public live-update mask correction).

Live transform aura application and removal now refresh `UNIT_FIELD_DISPLAYID` while preserving the race/gender native display, matching `AuraEffect::HandleAuraTransform`; player live update packets now include both display fields so self and nearby clients receive the transition (live transform display correction).

Character reputation loading now merges database visibility/war/inactive toggles with the DBC default flags and enforces the source hidden/invisible/peace-forced rules instead of replacing defaults with raw database flags. `tools/logincheck --self-check` covers visible default preservation and peace-forced war rejection, matching `ReputationMgr::Initialize`, `SetVisible`, `SetAtWar`, `SetInactive`, and `LoadFromDB` (reputation login-state correction).

Live player values now include `PLAYER_GUILDID` and `PLAYER_GUILDRANK`, allowing membership and rank changes to reach the owner and nearby clients through the source-public update mask, matching `Player::SetInGuild`, `Player::SetRank`, and `Guild::Member::ChangeRank` (live guild-field correction).

New character creation now persists watched faction `-1` and drunkenness `0`, matching `Player::Create` initialization; `tools/logincheck --self-check` guards both defaults (character-creation login-state correction).

Sanctuary area transitions now stop active player-vs-player attack state and clear the combat update fields for both participants, while creature combat remains intact, matching `Player::UpdateArea` and `CombatStopWithPets` (sanctuary PvP transition correction).

Loaded corpse restoration now applies `PLAYER_FIELD_BYTE_RELEASE_TIMER` only for non-instance corpse maps and leaves it clear for dungeon/raid/battleground maps, matching `Player::LoadCorpse`; the logincheck self-check covers the instance-type boundary (corpse login-state correction).

Loaded corpse replay now computes `SMSG_CORPSE_RECLAIM_DELAY` from the persisted corpse ghost timestamp and death-expire time, suppressing the packet after expiry, matching `Player::CalculateCorpseReclaimDelay(true)` (corpse reclaim packet correction).

Alive-player login now defers same-map persisted corpse conversion until after nearby visibility is sent, then emits the corpse destroy and optional bones create while removing the resurrectable row, matching `Map::AddPlayerToMap` and `Map::ConvertCorpseToBones` (stale-corpse login visibility correction).

Player selection now updates `UNIT_FIELD_TARGET` in self and nearby player values, persists through the initial player create state, and clears on teleport, matching `Player::SetSelection` and the public unit update field flags (selection update-field correction).

Persisted and runtime stealth auras now set and clear the source creep flag in `UNIT_FIELD_BYTES_1` and private stealth aura-vision bit in `PLAYER_FIELD_BYTES2`; live/create field coverage is included in logincheck, matching `AuraEffect::HandleModStealth` (stealth client-field correction).

Persisted and runtime invisibility auras now set and clear the private `PLAYER_FIELD_BYTES2` invisibility glow bit, matching `AuraEffect::HandleModInvisibility` and preserving the source client visibility state (invisibility client-field correction).

Persisted and runtime track-stealth auras now set and clear private `PLAYER_FIELD_BYTE_TRACK_STEALTHED` in `PLAYER_FIELD_BYTES`, matching `AuraEffect::HandleAuraTrackStealthed` and preserving the source client tracking state (track-stealth client-field correction).

Name queries now decode the raw `ObjectGuid` input used by `WorldSession::HandleNameQueryOpcode`, while retaining packed GUID output in `SMSG_NAME_QUERY_RESPONSE` (name-query wire correction).

Mobile transport gameobject uses now resolve the source `HighGuid::Mo_Transport` object before ordinary gameobject lookup, attach/detach player transport offsets, and retain the canonical transport GUID used by `Transport::AddPassenger` and movement handling (boat/zeppelin boarding correction).

Creature creates now expose template-derived maximum health separately from persisted current health, and combat target/motion state clamps stale persisted current health to that maximum, preserving `Creature::LoadFromDB` and `Unit::GetMaxHealth` semantics for critters and ordinary creatures (creature health-state correction).

Mounted auras now clear other mounted auras, keep the mount aura indefinite through login and runtime application, and dismount removes the actual mounted aura and client mount fields, matching `AuraEffect::HandleAuraMounted`, `Unit::Mount`, and `Unit::Dismount` (mount state correction).

Stealth, invisibility, stealth-detection, invisibility-detection, and stealth-level aura transitions now reconcile every loaded observer's player visibility set after the client fields change, emitting the corresponding create or out-of-range update, matching the source `AuraEffect` handlers' `Unit::UpdateObjectVisibility` calls (aura visibility transition correction).

Mounted and flight speed aura changes now recalculate source stack/non-stack modifiers at runtime, emit self and nearby run/flight speed packets, and set or unset client fly state when the final flight aura changes, matching `Unit::UpdateSpeed`, `Player::SetCanFly`, and the mounted-speed aura handlers (runtime mounted-speed correction).

Newly visible player and creature units now receive `SMSG_AURA_UPDATE_ALL` and active player melee `SMSG_ATTACK_START` after their create update, including login, worldport, movement streaming, and visibility-transition paths, matching `Player::SendInitialVisiblePackets` and `SendAurasForTarget` (visible-unit initial-state packet correction).

Login spell loading now rejects DBC-invalid race/class or level-ineligible active rows and deactivates lower non-stackable ranks when a higher `SkillLineAbility::SupercededBySpell` rank is active, matching `Player::_LoadSpells` plus `Player::AddSpell` rank-state behavior (initial spell-rank correction).

Persisted and reconstructed player skills now enforce the pinned `PLAYER_MAX_SKILLS = 127` client slot boundary across database rows, racial defaults, and `playercreateinfo_skills` additions, matching `Player::_LoadSkills` and preventing overflow into client update fields (skill-slot login correction).

Persisted and runtime `SPELL_AURA_MOD_CONFUSE` and `SPELL_AURA_MOD_FEAR` now set the source client control flags, interrupt active casts and melee, reject client movement/attacks while controlled, and clear each flag only after the final matching aura is removed, matching `Unit::SetControlled`, `SetConfused`, and `SetFeared` (fear/confuse control-state correction).

Player-target `SPELL_AURA_MOD_CHARM` now disables client control after login/worldport and runtime application, dismounts the target, interrupts casts/melee, rejects target input while the aura remains, and restores client control after the final charm aura is removed, matching the player-target portion of `Unit::SetCharmedBy`/`RemoveCharmedBy` (player charm-control correction).

Controlled player cast attempts now return the source `SPELL_FAILED_CHARMED`, `SPELL_FAILED_CONFUSED`, or `SPELL_FAILED_FLEEING` result instead of being silently consumed (controlled-cast failure correction).

Creature-target charm now marks the live creature player-controlled, changes its faction to the charmer, routes it through the existing pet follow/attack motion controller, clears combat, sends `SMSG_CLIENT_CONTROL_UPDATE` and a source-shaped `SMSG_PET_SPELLS` charm bar with creature react/command state, and restores the original creature state when the aura expires or is dispelled, matching normal `CHARM_TYPE_CHARM` in `Unit::SetCharmedBy`, `CharmSpellInitialize`, and `RemoveCharmedBy` (creature charm-control correction).

Player-backed vehicle enter/exit now emits `SMSG_PLAYER_VEHICLE_DATA` with the vehicle ID and zero removal state, matching the source vehicle-kit transition packet ownership (player vehicle packet correction).

Mounted-aura removal now emits the source `SMSG_DISMOUNT` packed-GUID transition after clearing `UNIT_FIELD_MOUNTDISPLAYID` and `UNIT_FLAG_MOUNT`; mount collision-height calculation remains dependent on the missing DBC model data (dismount packet correction).

Mount login/runtime collision-height packets now use the source `CreatureDisplayInfo.dbc` and `CreatureModelData.dbc` fields, including the `2.03128f` default and mounted-height formula, matching `Unit::GetCollisionHeight` and `SMSG_MOVE_SET_COLLISION_HGT` (DBC-backed mount collision correction).

`tools/logincheck --self-check` now exercises the mounted, unmounted, and default collision-height formula branches directly (collision-height fixture coverage).

Runtime `SPELL_AURA_FORCE_REACTION` apply/remove now rebuilds and sends `SMSG_SET_FORCED_REACTIONS`, matching `AuraEffect::HandleForceReaction` and `ReputationMgr::SendForceReactions` (live forced-reaction correction).

Login and manual social-list responses now filter deleted characters through the source character-cache join and enforce the 255-row bound, with a reduced-schema fallback (social login-list correction).

Friendly forced reactions now also stop the player’s current attack and hostile creature motions against the matching faction, matching `Unit::StopAttackFaction` (forced-reaction combat cleanup).

The current item-4 audit milestone hardens the reported character, transport, creature, and mount paths: name-query resolution now retains the requested GUID while checking both source raw and compatible packed forms; transport boarding uses a source-shaped movement heartbeat instead of the login movement-state payload; zero persisted creature health falls back to the computed live maximum when a combat target is reconstructed; and every mounted aura path forces one indefinite aura, clears competing mounts, resolves the mounted display, and recalculates movement state. A local world initialization run resolved the configured game-data fallback and loaded all 20 transport rows; client boarding and route movement remain runtime acceptance checks.

Creature pet casting now follows the source `HandlePetCastSpellOpcode` ownership boundary: active pet motion state retains all learned `pet_spell` IDs, charmed or owned motion is required, passive/unknown spells are rejected, explicit targets and DBC range are checked, cooldowns are tracked per spell, `SMSG_PET_CAST_FAILED` is returned for rejected requests, and direct damage, direct healing, triggered effects, and basic direct aura application execute with the creature GUID as caster. Combat logs, player victims, creature health, threat, and loot state are updated through the pet path. Complete area-target, dispel, mechanic-immunity, and full `Spell::CheckPetCast` parity remain open for later item-4 spell work.

The 2026-09-22 login/map source pass adds DBC-backed passive Death Knight rune conversions, full accepted movement-info serialization for player creates, source-derived movement and aura-multiplier fields, party-field visibility and same-map-instance isolation, and Eluna's map-player-enter callback after player-map-change. Game-data lookup validates required DBC files while resolving from the executable directory. `tools/transportcheck` reconstructs TaxiPathNode routes with source flags, delays, Catmull-Rom interpolation, acceleration, stop timing, and path periods; it validates all configured local transport routes against the world database and DBCs. That source pass originally left item 4 open; the login packet-order/update-mask gate was subsequently closed by the evidence below.

Item 4 closeout verification: the pinned `CharacterHandler.cpp`, `Player::SendInitialPacketsBeforeAddToMap`, and `Player::SendInitialPacketsAfterAddToMap` flow was compared with the Go login sequence and ordered packet/payload fixtures. The final recipient-specific group fields follow `Unit::BuildValuesUpdate` and `FactionTemplateEntry::IsFriendlyTo`; logincheck validates hostile cross-faction masking and unchanged friendly/disabled/self cases. The login trace checker rejects first occurrences of required packets before their source stage, verifies initial cinematics after pre-map packets, and bounds rune-order validation to pre-map so later runtime rune resyncs and cinematics remain valid. Adversarial fixtures cover early pre-map packets, premature world states/cinematics, valid runtime rune resync, and post-map gameplay cinematics. Full source, race, vet, logincheck, and build gates are recorded in the task ledger.

Item 5 active milestone: the earlier state bundle restores ranked default-skill tiers, reputation flags, death expiry, taxi routes/masks, group leadership, rested state, pet gates, nested-bag GUIDs, enchant durations, mail attachment fields, and periodic quest cooldowns. Follow-ups add DBC-backed item-limit-category, gem, and scaling data; item eligibility, stack/category caps, bag family/capacity, socket-gem uniqueness, bank-bag purchase gates, and atomic mail recovery; recursive prior spell ranks with source stackability and talent-rank selection; and saved quest item/player counters with exploration/timer/money/reputation completion checks plus zone-gated PvP credit. Logincheck exercises the ItemLimitCategory, ScalingStatDistribution, and spell-rank stackability DBC records. Remaining item-state gaps include exact quiver/equipment edge cases, live item-count transitions, group/raid PvP credit gates, and the real-character DB plus client packet replay diff; the local bin contains no recorded trace for that replay.
