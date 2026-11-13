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
-- Eluna creature events: 1 OnEnterCombat, 19 OnJustSummoned,
-- 31 OnQuestAccept. The C++ overrides no
-- KilledUnit / JustDied (no events 3/4); no SpellHit arm (event 14)
-- or MoveInLOS (event 27) arm exists for emily; the QuestAccept
-- (event 31) arm IS PORTED below (event 31 fires live from
-- engine/world/quest_details.go:311/607 + gossip.go:1361). DoAction
-- DOES have a bridge — event 38 (lua_creature_events.go:845, the
-- rocket-warhead precedent) — emily simply has no DoAction arm; the
-- old "no Eluna event at all" claim is retracted (2026-11-13).
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
-- is UNPORTED, not bridge-blocked (2026-11-13 correction): the escort
-- spine is live — StartEscort (lua_creature_events.go:2655),
-- MoveWaypoint (:4641), OnReachWP arrival ev6 (runtime.go:371, fired
-- creaturemotion.go:5313), MoveFollow (:5610), SummonCreature
-- (:4097), SetFaction (:6593), SetWalk (:3956), CastSpell (:954),
-- Kill (:3436), ExitVehicle (:3626), SetDeathState (:2212),
-- DespawnOrUnsummon (:7230), player:GroupEventHappens
-- (lua_player.go:5306). The single cast arm (DoCast 47184 on
-- Mrfloppy, :116) is waypoint-18-gated, not timer-driven
-- (thassarian/borean_tundra precedent). STILL BLOCKED: waypoint 19
-- EnterVehicle has no bridge (no methods["EnterVehicle"] in the
-- engine); waypoint 20 HandleEmoteCommand needs a per-leg
-- PerformEmote (:4475) twin check at port time. OnQuestAccept Talk
-- SAY_QUEST_ACCEPT 11 is PORTED below (event 31).
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
-- npc_wounded_skirmisher (Overwhelmed 12288): PORTED round 21
-- (2026-11-13) in lua_scripts/northrend/zone_grizzly_hills.lua:347-388
-- — SpellHit 48812 -> Talk(SAY_RANDOM) + DoCast 48813 + MovePoint /
-- despawn legs (ev14 + ev28). Entry 27463 DB-verified: TDB +
-- AC creature_template rows bind ScriptName 'npc_wounded_skirmisher'
-- (re-verified 2026-11-13). The old "no entry constant, unverifiable,
-- entry-blocked queue" wording is retracted — the DB-verification
-- ruling governs (a creature_template row carrying the exact C++
-- ScriptName IS the binding evidence); no C++ entry constant exists,
-- but none is needed.
-- npc_venture_co_straggler (Smoke 'Em Out 12323/12324): PORTED round 21
-- (2026-11-13) in lua_scripts/northrend/zone_grizzly_hills.lua:390-509
-- — SpellHit 49075 -> pacify + CreateLuaEvent chain (credit cast
-- 49093, Talk SAY_SEO, three MovePoint legs, DisappearAndDie twin)
-- + EVENT_CHOP (DoCastVictim SPELL_CHOP 43410, JustEngagedWith init
-- (3s,6s), Repeat(10s,12s)). Entry 27570 DB-verified: TDB + AC
-- creature_template rows bind ScriptName 'npc_venture_co_straggler'
-- (re-verified 2026-11-13). The old "unbridged MovePoint /
-- DisappearAndDie / SetImmuneToPC chain" and "no entry, unverifiable"
-- wordings are retracted — MoveTo / DespawnOrUnsummon / SetFlag
-- (59, 0x100) are all live, and the DB-verification ruling governs.
-- npc_lake_frog: Talk SAY_MAIDEN_0 / SAY_MAIDEN_1 ride
-- EVENT_LAKEFROG_2/5 timers (SetEntry 33220 maiden transform,
-- SPELL_MAIDEN_OF_ASHWOOD_LAKE_TRANSFORM 62550, gossip-flag
-- toggling, DespawnOrUnsummon) — the EVENT_LAKEFROG_1..5 transform
-- sequence IS PORTED (2026-10-24) in zone_grizzly_hills.lua
-- (lakeFrogChain1-5, CreateLuaEvent pumps; UpdateEntry, SetFlag /
-- RemoveFlag, Talk all live); the old "no bridge for the sequence
-- machine" claim is retracted. The kissed-frog MoveFollow leg
-- (zone_grizzly_hills.cpp:765) stays unported — MoveFollow IS bridged
-- (lua_creature_events.go:5610), so that leg is unported work, not
-- bridge-blocked. ReceiveEmote TEXT_EMOTE_KISS + item 44986 /
-- auras 62537/62574/62581 legs are PORTED in
-- lua_scripts/northrend/zone_grizzly_hills.lua (lakeFrogReceiveEmote,
-- event 8 on 33211/33224, 2026-10-23) — the stale "no-emote / no-item
-- bridge" claim is retracted; not duplicated here (two handlers would
-- double-fire, see note at end of file).
-- OnGossipSelect (zone_grizzly_hills.cpp:777-781: ungated DoCast(player,
-- SPELL_SUMMON_ASHWOOD_BRAND 62554)) is PORTED in zone_grizzly_hills.lua
-- (lakeFrogGossipSelect) — no menuId or gossipListId gate in C++, so the
-- Lua port gates on nothing either.
-- npc_rocket_propelled_warhead + spell_z_check / spell_warhead_fuse:
-- PORTED 2026-11-05 in lua_scripts/northrend/
-- npc_rocket_propelled_warhead.lua (DoAction bridge event 38,
-- RegisterVehicleEvent OnAddPassenger, ev14/ev24 arms) — the
-- "no bridges" claim above is retracted for those three; the
-- remaining spell loaders (spell_vehicle_warhead_fuse /
-- spell_warhead_detonate) stay SpellScript-only — spell_shredder_delivery
-- is PORTED in lua_scripts/northrend/zone_grizzly_hills.lua
-- (RegisterSpellEvent 48610 ev5); the "no SpellScript bridge anywhere in
-- the model" claim was stale (boss_xevozz.lua:372 ports the identical
-- OnEffectHitTarget arm). spell_infected_worgen_bite
-- is an AuraScript, not a SpellScript — PORTED in
-- lua_scripts/northrend/zone_grizzly_hills.lua (RegisterAuraEvent
-- 53094 ev2/ev4, max-stack Remove + self-cast 53095).

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

-- C++ npc_emilyAI::JustSummoned (zone_grizzly_hills.cpp:70-76): a
-- creature Emily summons attacks Mrfloppy (26589) if one is within
-- 50yd, else Emily's current victim. EngageWithTarget is the
-- AttackStart twin (midnight precedent); a nil victim means no
-- engage (C++ AttackStart on a null victim is a no-op in practice).
-- LATENT: Emily's summons ride the unported WaypointReached escort
-- machine (waypoints 10/17 summon 26586/26590), so nothing summons
-- through her in the Go model yet; the arm is wired so the escort
-- port inherits it (nesingwary-trapper latent-AI precedent).
local function emilyJustSummoned(event, creature, summon)
    local mrfloppy = creature:FindNearestCreature(ENTRY_MRFLOPPY, 50.0)
    if mrfloppy ~= nil then
        summon:EngageWithTarget(mrfloppy)
    else
        local victim = creature:GetVictim()
        if victim ~= nil then
            summon:EngageWithTarget(victim)
        end
    end
end

RegisterCreatureEvent(ENTRY_EMILY, 19, emilyJustSummoned)

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
