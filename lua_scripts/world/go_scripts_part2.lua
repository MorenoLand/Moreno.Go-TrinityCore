-- go_scripts.cpp PART 2 (classes at C++ :683-1363; AddSC :1375-1399) — triage notes.
-- Sibling child owns :61-682. 9 of the 10 classes below are DOCUMENTED-ONLY gap
-- classes; go_midsummer_ribbon_pole is PORTED (Register call at file end).
-- Every other arm is gated on at least one unbridged call, and 9 of
-- the 10 classes bind by ScriptName with no entry ID named anywhere in the
-- C++ tree (whole-tree quoted-name grep: go_scripts.cpp is the sole hit),
-- so the gameobject_template binding stays DB-side per the no-invent rule.
-- Bridge facts (grepped this run against the CURRENT runtime):
--   - GameObjectEventOnAIUpdate (=1, runtime.go) has NO fire site in
--     engine/ — UpdateAI arms (toy_train, *_music, bells) have no trigger
--     (re-verified 2026-10-24: zero fire sites outside the const decl).
--   - IsHolidayActive IS bridged 2026-10-24 (lua_objects.go; cached
--     active-holiday set, GameEventMgr.cpp:1847 twin) — the *_music
--     holiday gates are now portable; AIUpdate fire site still blocks.
--   - player:GetQuestStatus IS bridged since 2026-10-24 (lua_player.go;
--     veil_skith_cage gate now portable, frostblade_shrine now
--     bridge-complete (entry DB-side only).
--   - CastSpell: creature surface (lua_creature_events.go:62) +
--     player surface bridged 2026-10-24 (lua_player.go:309); no
--     gameobject:CastSpell in Go (corrected 2026-10-24: C++ DOES have a
--     GO caster path — WorldObject::CastSpell, Object.h:467 — but Go's
--     cast pipeline is session/creature executors only, no GO-caster
--     executor; blocks toy_train pulse only — bonfire,
--     shrine, ribbon-pole casts are player casts, bridged).
--   - SetStandState: creature surface bridged (lua_creature_events.go:1621)
--     + player surface bridged 2026-10-24 (lua_player.go); shrine sit
--     now portable.
--   - CloseGossipMenuFor: no literal Lua method, but the twin
--     player:GossipClearMenu + player:GossipComplete IS bridged
--     (lua_player.go:581-582, vaelastrasz precedent); bonfire select
--     tail portable via twin.
--   - GameObject grid search: creature LIST search
--     (GetCreatureListWithEntryInGrid) IS bridged 2026-10-24
--     (lua_objects.go, creature-bridge twin) — cage child search now
--     portable; GetPlayerListInGrid IS bridged 2026-10-24
--     (lua_objects.go, session-enumeration twin: alive-only, same
--     map+instance, 3D + 1.5 player reach) — brewfest SHATTRATH +
--     midsummer per-player dispatch now portable; ribbon-pole bunny is a
--     single-creature search, bridged — see below);
--     corrected 2026-10-24 — single-creature player:FindNearestCreature
--     IS bridged since 2026-10-24 (lua_player.go), but it returns one
--     nearest creature, not the list the cage arm iterates; GO-surface
--     FindNearestCreature bridged 2026-10-24 too (lua_objects.go), so
--     the ribbon-pole bunny search is portable (sole block DoAction).
--   - Creature DoAction / MovePoint / AI()->Talk via a GO-obtained
--     creature: no bridge (blocks cage children walk+Talk, ribbon pole
--     ACTION_COSMETIC_FIRES) — re-verified 2026-10-24: zero DoAction
--     hits in runtime.go + lua_creature_events.go, and bunny 17066 has
--     no AI anywhere else in the C++ tree (go_scripts.cpp is its sole
--     hit), so even a dispatch bridge would have no handler to call.
--   - GameObject GetAreaId/GetZoneId IS bridged 2026-10-24
--     (lua_objects.go, zoneAndAreaID twin) — brewfest area dispatch and
--     bells zone->soundId init now portable; remaining blocks are the
--     holiday-adjacent legs and fire sites, not area/zone.
--   - No game-event hook in runtime.go at all — go_bells OnGameEvent(73)
--     has no fire path (re-verified 2026-10-24: zero OnGameEvent hits
--     in runtime.go + gameobject_hooks.go).
--   - Bridged but unreachable: go:UseDoorOrButton([delay]) (lua_objects.go),
--     player:HasAura (lua_objects.go:228), go:PlayMusic(id[, player]) =
--     SMSG_PLAY_MUSIC with optional per-player target (lua_objects.go:106),
--     go:PlayDirectSound(id) (lua_objects.go:120), GetGameTime() unix
--     seconds (runtime.go:768). These become usable if the gating bridges
--     above ever land.

-- DOCUMENTED-ONLY: go_veil_skith_cage (C++ :683-716)
-- OnGossipHello (:691-709): me->UseDoorOrButton() is bridged, but the arm
-- is entry-less in-tree (ScriptName "go_veil_skith_cage", DB-side bind) and
-- the quest arm needs player:GetQuestStatus(QUEST_MISSING_FRIENDS 10852 —
-- enum :678) == QUEST_STATUS_INCOMPLETE (bridged 2026-10-24), grid search for
-- NPC_CAPTIVE_CHILD 22314 (enum :679, bridged 2026-10-24 via
-- go:GetCreaturesInGrid), player:KilledMonsterCredit
-- (bridged), creature DespawnOrUnsummon/MovePoint/AI()->Talk(SAY_FREE_0 0,
-- enum :680) (no bridge) — sole blocks now MovePoint/Talk/DoAction.

-- DOCUMENTED-ONLY: go_frostblade_shrine (C++ :730-758)
-- OnGossipHello (:739-753): me->UseDoorOrButton(10) bridged (delay arg);
-- player:HasAura(SPELL_RECENT_MEDITATION 61720) bridged and
-- player:GetQuestStatus(QUEST_THE_CLEANSING_HORDE 11317 /
-- QUEST_THE_CLEANSING_ALLIANCE 11322, enum :733-736) bridged since
-- 2026-10-24 — but the cleansing arm still needs
-- player:CastSpell(player, SPELL_CLEANSING_SOUL 43351) bridged
-- 2026-10-24 (lua_player.go:309); player:SetStandState bridged
-- 2026-10-24 (lua_player.go) — the arm is now bridge-complete, entry
-- DB-side only.

-- DOCUMENTED-ONLY: go_midsummer_bonfire (C++ :769-800)
-- OnGossipSelect (:783-788; gameobject_gossip ON_SELECT=2 fires per
-- engine/world/gameobject_hooks.go:215): player:CastSpell(player,
-- STAMP_OUT_BONFIRE_QUEST_COMPLETE 45458, true — enum :764) bridged
-- 2026-10-24 (lua_player.go:309); CloseGossipMenuFor(player) rides the
-- bridged twin player:GossipClearMenu + player:GossipComplete
-- (lua_player.go:581-582) — bridge-complete, entry DB-side only.

-- go_midsummer_ribbon_pole (C++ :808-834) — PORTED 2026-10-24.
-- OnGossipHello (:817-826): me:FindNearestCreature(
-- NPC_POLE_RIBBON_BUNNY 17066, 10.0f) bridged 2026-10-24 (lua_objects.go),
-- player:CastSpell(player, RibbonPoleSpells[urand(0,2)] in {29705, 29726,
-- 29727} — enum :811-813) bridged 2026-10-24. The DoAction(
-- ACTION_COSMETIC_FIRES 0) leg has no dispatch bridge, so the start
-- rides the creature:SetData relay: the handler writes the bunny's
-- running latch (SetData key 1, shared with npcs_special_part1.lua)
-- and the bunny's event-7 detector starts the ring chain.
-- Entry IS in-tree (GO_RIBBON_POLE 181605, npcs_special.cpp enum), so
-- the GO registers — the old "Entry DB-side" ground is retracted.
-- C++ returns true (handled) == Lua true (native use suppressed).
local RIBBON_POLE_SPELLS = { 29705, 29726, 29727 }

local function onRibbonPoleGossip(_, player, go)
    local bunny = go:FindNearestCreature(17066, 10)
    if bunny ~= nil then
        if bunny:GetData(1) ~= 1 then
            bunny:SetData(1, 1)
        end
        player:CastSpell(player, RIBBON_POLE_SPELLS[math.random(3)], true)
    end
    return true
end

-- DOCUMENTED-ONLY: go_toy_train_set (C++ :839-869)
-- UpdateAI (:851-860): GameObjectEventOnAIUpdate has no fire site
-- (primary block — the pulse never fires); secondary blocks are
-- gameobject:CastSpell(nullptr, SPELL_TOY_TRAIN_PULSE 61551, true —
-- enum :842) (still unbridged; creature/player surfaces only) and the
-- DoAction (:863-867, "wrecker'd") -> me->Delete() leg (no GO DoAction
-- dispatch; Despawn/RemoveFromWorld hide, not delete). Entry DB-side.

-- DOCUMENTED-ONLY: go_brewfest_music (C++ :917-1056)
-- UpdateAI (:931-1051): no AIUpdate fire site; EVENT_BM_SELECT_MUSIC /
-- EVENT_BM_START_MUSIC machine is gated on IsHolidayActive(HOLIDAY_BREWFEST)
-- (bridged 2026-10-24), area dispatch on me->GetAreaId() (bridged
-- 2026-10-24; areas
-- SILVERMOON 3430 / UNDERCITY 1497 / ORGRIMMAR_1 1296 / ORGRIMMAR_2 14 /
-- THUNDERBLUFF 1638 / IRONFORGE_1 809 / IRONFORGE_2 1 / STORMWIND 12 /
-- EXODAR 3557 / DARNASSUS 1657 / SHATTRATH 3703 — enum :892-905) with
-- SHATTRATH leg now rides bridged GetPlayerListInGrid (2026-10-24) +
-- bridged per-player PlayMusic — sole block now AIUpdate fire site.
-- go:PlayMusic(soundId[, player]) IS bridged (SMSG_PLAY_MUSIC) and maps
-- 1:1 onto me->PlayDirectMusic (sounds 11810/11812/11813 goblin,
-- 11811/11814/11815 dwarf — enum :879-887; durations 95s/155s/23s/68s/93s/28s
-- — :889-894), but is unreachable behind the holiday + area gates.
-- Entry DB-side.

-- DOCUMENTED-ONLY: go_midsummer_music (C++ :1074-1122)
-- UpdateAI (:1086-1114): no fire site; gated on
-- IsHolidayActive(HOLIDAY_FIRE_FESTIVAL) (bridged 2026-10-24) +
-- GetPlayerListInGrid (bridged 2026-10-24). Per-player go:PlayMusic(id, player)
-- IS bridged and matches
-- me->PlayDirectMusic(EVENTMIDSUMMERFIREFESTIVAL_A 12319 /
-- EVENTMIDSUMMERFIREFESTIVAL_H 12325 — enum :1063-1067) exactly, but is
-- unreachable. Entry DB-side.

-- DOCUMENTED-ONLY: go_darkmoon_faire_music (C++ :1139-1176)
-- UpdateAI (:1149-1171): no fire site; gated on IsHolidayActive(
-- HOLIDAY_DARKMOON_FAIRE_ELWYNN / _THUNDER / _SHATTRATH) (bridged
-- 2026-10-24) — sole block now the AIUpdate fire site.
-- go:PlayMusic(MUSIC_DARKMOON_FAIRE_MUSIC 8440 — enum :1129-1132) bridged
-- but unreachable. Entry DB-side.

-- DOCUMENTED-ONLY: go_pirate_day_music (C++ :1193-1230)
-- UpdateAI (:1203-1225): no fire site; gated on
-- IsHolidayActive(HOLIDAY_PIRATES_DAY) (bridged 2026-10-24) — sole block
-- now the AIUpdate fire site.
-- go:PlayMusic(MUSIC_PIRATE_DAY_MUSIC 12845 — enum :1183-1186) bridged but
-- unreachable. Entry DB-side.

-- DOCUMENTED-ONLY: go_bells (C++ :1273-1363)
-- Entries ARE in-tree (enum :1252-1256): GO_HORDE_BELL 175885,
-- GO_ALLIANCE_BELL 176573, GO_KHARAZHAN_BELL 182064.
-- InitializeAI (:1282-1325): needs me:GetEntry() (bridged) + me:GetZoneId()
-- (bridged 2026-10-24) to pick _soundId from BELLTOLLHORDE 6595 /
-- BELLTOLLTRIBAL 6675
-- / BELLTOLLALLIANCE 6594 / BELLTOLLNIGHTELF 6674 / BELLTOLLDWARFGNOME 7234 /
-- BELLTOLLKHARAZHAN 9154 (enum :1234-1241; zones 85/1497/267/10/1/1537/
-- 141/1657/331 — enum :1243-1250).
-- OnGameEvent (:1327-1344, GAME_EVENT_HOURLY_BELLS 73 — enum :1258): no
-- game-event hook exists in runtime.go — no fire path at all (verified
-- this run; sole blocks now game-event hook + AIUpdate fire site).
-- UpdateAI (:1346-1360): no AIUpdate fire site; ring arm is
-- go:PlayDirectSound(_soundId) which IS bridged (lua_objects.go:120), and
-- GetGameTime() exists, but the trigger and the zone init are dead, so
-- nothing is registered.

RegisterGameObjectGossipEvent(181605, 1, onRibbonPoleGossip)
