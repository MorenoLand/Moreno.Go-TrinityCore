-- Emily (Grizzly Hills) --
-- Lua port of src/server/scripts/Northrend/zone_grizzly_hills.cpp
-- (npc_emilyAI — JustEngagedWith arm only). Zone-script unit per
-- northrend_script_loader.cpp order (dragonblight done; grizzly_hills:
-- npc_mrfloppy, npc_outhouse_bunny, npc_tallhorn_stag,
-- npc_amberpine_woodsman, npc_wounded_skirmisher,
-- npc_venture_co_straggler, npc_lake_frog, npc_rocket_propelled_warhead,
-- spell_shredder_delivery, spell_infected_worgen_bite,
-- spell_vehicle_warhead_fuse, spell_warhead_detonate, spell_z_check,
-- spell_warhead_fuse documented-only).
-- Entry (Floppy enum, verifiable from the C++ sources): 26588
-- (NPC_EMILY). The creature_template ScriptName binding is DB-side
-- (no TDB in this workspace).
-- Eluna creature events: 1 OnEnterCombat. The C++ overrides no
-- KilledUnit / JustDied (no events 3/4); no SpellHit arm (event 14,
-- runtime.go:195-214), no MoveInLOS (event 27) or QuestAccept (event
-- 31) arm for emily is PORTED below (event 31 fires live from
-- engine/world/quest_details.go); DoAction has no Eluna event at all
-- — no bridge;
-- no OnReset work worth wiring (GUID clears only).
-- Fight shape (C++-exact for all modeled arms):
-- OnEnterCombat(1): Talk(SAY_RANDOMAGGRO 4, no target — C++ calls
-- Talk(4) with no who) — the event-1 auriaya precedent. The AI
-- class is EscortAI-based, but the JustEngagedWith arm is a bare
-- self-Talk, fully covered by the event-1 Talk bridge.
-- Unmodeled (no bridges — documented, not wired):
-- npc_emily: WaypointReached escort cinematic (Talk SAY_WORGHAGGRO1
-- 0 / SAY_WORGRAGGRO3 2 / SAY_VICTORY2 6 / SAY_VICTORY3 7 /
-- SAY_VICTORY4 8 / SAY_QUEST_COMPLETE 12 + SummonCreature
-- 26586/26590 + DoCast 47184 + EnterVehicle + faction swaps +
-- MovePoint/MoveFollow + DisappearAndDie legs) — the 27-waypoint
-- EscortAI cinematic (WaypointReached, zone_grizzly_hills.cpp:81-175)
-- has no escort-waypoint bridge; the single cast arm (DoCast 47184 on
-- Mrfloppy, :116) is waypoint-18-gated, not timer-driven
-- (thassarian/borean_tundra precedent); OnQuestAccept Talk
-- SAY_QUEST_ACCEPT 11 is PORTED below (event 31) — the Start escort
-- + Mrfloppy MoveFollow legs stay out (no bridges).
-- npc_mrfloppy: JustEngagedWith Talk arms are all cross-creature
-- (Emily->AI()->Talk SAY_WORGHAGGRO2 1 / SAY_WORGRAGGRO4 3 /
-- SAY_RANDOMAGGRO 4, entry-gated on 26586/26590) — PORTED below via
-- the cross-creature Talk bridge (FindNearestCreature returns the
-- Emily creature object; Talk is called on it — the illidari-council
-- memberTargetDied precedent); the stale "no cross-creature Talk
-- bridge" claim is retracted. No JustDied / KilledUnit arms.
-- npc_outhouse_bunny (Doing Your Duty 12227): PORTED in
-- npc_outhouse_bunny.lua (entry 27326, go_scripts.cpp:585) — SpellHit
-- SPELL_OUTHOUSE_GROANS 48382 -> counter arm -> DoCast(caster,
-- SPELL_CAMERA_SHAKE 47533) when counter < 5 / DoCast(me,
-- SPELL_DUST_FIELD 48329) always (zone_grizzly_hills.cpp:298-323);
-- the gender-gated sound arm (SetData, Type 1): the bunny-side
-- Reset SetData(1,0) + gender-gated PlayDirectSound legs ARE ported
-- in npc_outhouse_bunny.lua (2026-10-24) — the stale "no SetData
-- bridge" wording is retracted; only the go_amberpine_outhouse
-- gossip caller (which writes the gender) stays out.
-- npc_tallhorn_stag — PORTED in lua_scripts/northrend/
-- zone_grizzly_hills.lua (2026-10-23): the Reset/UpdateAI _phase haunch
-- arm via the new creature:FindNearestGameObject bridge.
-- npc_amberpine_woodsman (Amberpine woodsman skinning event): zero Talk;
-- UpdateAI UNIT_NPC_EMOTESTATE state machine (0.2f FindNearestCreature
-- 26363) — unregistered AI (no C++ entry constant), stays out.
-- npc_wounded_skirmisher (Overwhelmed 12288): SpellHit
-- SPELL_RENEW_SKIRMISHER 48812 -> Talk(SAY_RANDOM) + DoCast
-- SPELL_KILL_CREDIT 48813 — bridgeable (event 14 SpellHit + cast), but
-- the skirmisher carries NO entry constant anywhere in the C++ tree
-- (DB ScriptName binding only, unverifiable — laughing-skull
-- precedent), so no entry-verifiable registration can be wired.
-- npc_venture_co_straggler (Smoke 'Em Out 12323/12324): Talk(SAY_SEO)
-- rides EVENT_STRAGGLER_2 timer + MovePoint chain + DisappearAndDie;
-- SpellHit SPELL_SMOKE_BOMB 49075 -> pacify + event chain — the
-- SpellHit arm itself bridges now (event 14), but the smoke-bomb arm
-- is gated on the unbridged MovePoint/DisappearAndDie/SetImmuneToPC
-- chain, and the straggler carries NO entry constant anywhere in
-- the C++ tree (DB ScriptName binding only, unverifiable), so no
-- entry-verifiable registration can be wired. EVENT_CHOP
-- (DoCastVictim SPELL_CHOP 43410, 2-arg non-triggered,
-- JustEngagedWith init (3s,6s), unconditional Repeat(10s,12s),
-- zone_grizzly_hills.cpp:572-574/:610-616) is SEPARABLE from the
-- SpellHit chain (scheduled in JustEngagedWith independently, not
-- gated on the SpellHit flip) and would bridge in isolation — but
-- no entry (see above), so the full spec stays documented-only.
-- npc_lake_frog: Talk SAY_MAIDEN_0 / SAY_MAIDEN_1 ride
-- EVENT_LAKEFROG_2/5 timers (SetEntry 33220 maiden transform,
-- SPELL_MAIDEN_OF_ASHWOOD_LAKE_TRANSFORM 62550, gossip-flag
-- toggling, DespawnOrUnsummon) — the transform sequence rides
-- EVENT_LAKEFROG timers + entry-update/gossip legs with no bridge
-- for the sequence machine; ReceiveEmote TEXT_EMOTE_KISS + item 44986 /
-- auras 62537/62574/62581 legs are PORTED in
-- lua_scripts/northrend/zone_grizzly_hills.lua (lakeFrogReceiveEmote,
-- event 8 on 33211/33224, 2026-10-23) — the stale "no-emote / no-item
-- bridge" claim is retracted; not duplicated here (two handlers would
-- double-fire, see note at end of file).
-- OnGossipSelect (zone_grizzly_hills.cpp:777-781: ungated DoCast(player,
-- SPELL_SUMMON_ASHWOOD_BRAND 62554)) is PORTED below — no menuId or
-- gossipListId gate in C++, so the Lua port gates on nothing either.
-- npc_rocket_propelled_warhead: VehicleAI PassengerBoarded DoCast
-- SPELL_VEHICLE_WARHEAD_FUSE + DoAction MovePoint chain — no
-- PassengerBoarded / no-DoAction / no-movement bridges.
-- spell_shredder_delivery / spell_infected_worgen_bite /
-- spell_vehicle_warhead_fuse / spell_warhead_detonate /
-- spell_z_check / spell_warhead_fuse: all SpellScript/AuraScript
-- — no SpellScript/AuraScript bridge anywhere in the model
-- (sindragosa ice_tomb_target precedent).

local ENTRY_EMILY = 26588
local ENTRY_MRFLOPPY = 26589 -- zone_grizzly_hills.cpp:36 (NPC_MRFLOPPY)
local NPC_HUNGRY_WORG = 26586 -- zone_grizzly_hills.cpp:37
local NPC_RAVENOUS_WORG = 26590 -- zone_grizzly_hills.cpp:38
local SAY_WORGHAGGRO2 = 1 -- He's going for Mr. Floppy!
local SAY_WORGRAGGRO4 = 3 -- He's gonna eat Mr. Floppy! You gotta help Mr. Floppy! You just gotta!
local SAY_RANDOMAGGRO = 4 -- There's a big meanie attacking Mr. Floppy! Help!
local SAY_QUEST_ACCEPT = 11 -- Are you ready, Mr. Floppy? Stay close to me and watch out for those wolves!

local QUEST_PERILOUS_ADVENTURE = 12027

-- C++ JustEngagedWith: Talk(SAY_RANDOMAGGRO) (no target).
local function emilyEnterCombat(event, creature, target)
    creature:Talk(SAY_RANDOMAGGRO)
end

-- C++ OnQuestAccept (zone_grizzly_hills.cpp:186-195): quest 12027
-- -> Talk(SAY_QUEST_ACCEPT), untargeted. The Mrfloppy MoveFollow and
-- EscortAI::Start legs have no bridges (muglash/ranshalla precedent).
local function emilyQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_PERILOUS_ADVENTURE then
        return
    end
    creature:Talk(SAY_QUEST_ACCEPT)
end

RegisterCreatureEvent(ENTRY_EMILY, 1, emilyEnterCombat)
RegisterCreatureEvent(ENTRY_EMILY, 31, emilyQuestAccept)

-- C++ npc_mrfloppyAI::JustEngagedWith (zone_grizzly_hills.cpp:223-235):
-- GetClosestCreatureWithEntry(me, NPC_EMILY 26588, 50.0f); switch on
-- the attacker's entry: HUNGRY_WORG 26586 -> Emily Talks
-- SAY_WORGHAGGRO2 (1); RAVENOUS_WORG 26590 -> SAY_WORGRAGGRO4 (3);
-- default -> SAY_RANDOMAGGRO (4). The Talk is Emily's (the found
-- creature object talks); the nil-guard mirrors C++'s
-- `if (Creature* Emily = ...)` (twin-emperors FindNearestCreature
-- precedent).
local function mrfloppyEnterCombat(event, creature, target)
    creature:SetMeleeAttack(false) -- zone_grizzly_hills.cpp:245: custom UpdateAI never calls DoMeleeAttackIfReady (victim-check only in C++, never melees)
    local emily = creature:FindNearestCreature(ENTRY_EMILY, 50.0)
    if emily == nil then
        return
    end
    if target == nil then
        emily:Talk(SAY_RANDOMAGGRO)
        return
    end
    local attackerEntry = target:GetEntry()
    if attackerEntry == NPC_HUNGRY_WORG then
        emily:Talk(SAY_WORGHAGGRO2)
    elseif attackerEntry == NPC_RAVENOUS_WORG then
        emily:Talk(SAY_WORGRAGGRO4)
    else
        emily:Talk(SAY_RANDOMAGGRO)
    end
end

RegisterCreatureEvent(ENTRY_MRFLOPPY, 1, mrfloppyEnterCombat)

-- (npc_lake_frog gossip-select for 33211 was a duplicate of the canonical
-- port in zone_grizzly_hills.lua; removed 2026-10-22 — both handlers fired and
-- cast SPELL_SUMMON_ASHWOOD_BRAND 62554 twice per gossip select.)
