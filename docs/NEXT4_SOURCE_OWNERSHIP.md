# Next4 source ownership and acceptance matrix

This matrix is based on the pinned TrinityCore checkout at `dcdbc0c5d88eb96f412f69c34bd5b9de2eed5df6`, the current Go tree, and executable evidence paths. Historical local test evidence was removed from the public tree by policy; source compilation or a self-check is not a behavioral acceptance claim.

| No. | Pinned reference owner | Go owner | Current evidence | Acceptance |
| ---: | --- | --- | --- | --- |
| 1 | repository commit, source inventory, and Next3 reconciliation | `docs/PARITY.md`, `docs/PARITY_COVERAGE.md`, Next4 evidence log | baseline and pinned commit recorded | recorded |
| 2 | all source areas listed by `src/server`, `src/tools`, `src/server/scripts`, and `tests` | this matrix plus `tools/parity`, `tools/scriptinventory` | every numbered owner is listed here | matrix complete; behavior open |
| 3 | `src/server/game/Server/WorldSocket.cpp`, `WorldSession.cpp`, and packet handlers | `docs/NEXT4_PACKET_STATE_REPLAY.md`, `pkg/protocoltrace`, `tools/packettrace`, `engine/world/server.go`, `engine/auth/trace.go`, `engine/service/service.go`, `engine/database/database.go` | source-defined JSON packet replay, failure replay, and redacted database transition results pass | complete for source-defined replay; client/login state evidence remains in items 4–5 |
| 4 | `Handlers/CharacterHandler.cpp`, `Entities/Player/Player.cpp::SendInitialPacketsBeforeAddToMap/AfterAddToMap`, `Maps/Map.cpp::SendInitSelf/AddPlayerToMap`, `Maps/Map.cpp::GetZoneAndAreaId` | `engine/world/characters.go`, `worldstates.go`, `terrain.go`, `player_packets.go`, `player_state.go`, `transports.go`, `death.go`, `skills.go`, `guild.go`, `dynamic_spells.go`, `tools/logincheck`, `tools/terraincheck`, `tools/worldstatecheck` | source-defined login order, single visibility pass (`879320e`), deterministic talent order (`f505235`), attached/map transport create phases (`35f3d8d`), attached transport passenger create batch (`5a30b16`), player GUID low-word update-mask correction and source-order follow-up (`901c330`), `PLAYER_CHOSEN_TITLE` field correction (`3307faa`), public player update-field flag correction (`939d6fb`), create-block field verifier (`c8dbd96`), mandatory achievement/equipment login-order stages (`1b0473a`), map 530/zone 3430 world-state fixture (`708486c`), hidden-duration aura packet correction (`a0a25e6`), live-aura hidden-duration correction (`0a9d5e9`), persisted pet-aura hidden-duration correction (`49f74ab`), aura effect-mask correction (`19dedcb`), runtime aura effect-index correction (`080f4e6`), single aura-update effect-index correction (`bb64c14`), packed achievement timestamps (`5dad1ad`), post-load achievement completion timing (`cb7dfeb`), hidden achievement visibility (`fc28f25`), creature cast-speed/hover-height fields (`bca9cf4`), creature query fallback correction (`1c4343c`), transport dynamic path progress (`10ca358`), transport route period (`2e078a9`), transport passenger hover height (`bc57f4d`), supplied September login trace correlation (pre-verify `SMSG_CRITERIA_UPDATE`), DBC reputation-list mapping (`286d282`), active pet create fields (`79e5df9`), pet ActiveStates preservation (`47cd644`), guild roster state (`0c74b76`), source login time speed (`a47bf9d`), regression verifier for pre-verify achievement packets (`b1deed4`), inventory create-field boundary (`e9e239b`), packed login time (`8506995`), ready rune resync (`118cd98`), triggered login effect (`c69879c`), cinematic branch (`50caa6b`), time-sync schedule (`a3ccee0`), guild bank login packet (`ba58ec7`), guild signed-on event (`8fe0b3b`), guild MOTD event (`6a88a62`), saved player appearance fields and source-shaped empty guild-event parameters (`80ed6b9`), nearby-player public visibility (`278c4bb`), reciprocal player create broadcast (`351037c`), stateful player visibility/removals (`0c2de4e`), dynamic spell object login visibility (`a7206c4`), nearby corpse/bones visibility (`2397643`), recorded login order/create-block verifier (`ef02ca3`), selected-title compatibility guard (`225064a`), declined-name query payload (`0f87368`), group HighGuid encoding (`a3e0100`), group `SMSG_GROUP_LIST` GUID/layout validator (`3d10e2d`), persisted LFG dungeon/state login fields (`3ebf379`), separate runtime/database group GUID loading (`8f3f574`), source member-order preservation (`a21c0d8`), character customization-flag precedence (`67ca009`), difficulty, cooldown, unlearn, movement, corpse, terrain/WMO area, and world-state fixtures | open: complete source-defined packet/update-mask comparison |
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
| 15 | `Handlers/ChatHandler.cpp`, `Chat/Channel.cpp`, `Chat/Chat.cpp` | `engine/world/chat.go`, `channels.go`, `social.go` | source-defined chat/channel and encrypted chat fixtures; guild/officer speak/listen rights now use the pinned `GR_RIGHT_*CHAT*` bit values (`63f675b`); client-sent universal language is rejected before permission-based server language rewriting (`e14c976`); addon chat is gated by the source `AddonChannel` setting (`0d53d23`), bypasses normal-text control-character rejection (`62b6183`), and does not route through normal commands (`cf35c4e`) | open: all channel/language/addon routing verification |
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

The first open behavioral gate remains the complete source-defined login packet/update-mask/state comparison in items 4–5; later rows are not promoted by inventory counts.

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
