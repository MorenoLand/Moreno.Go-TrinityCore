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
-- WaypointReached escort cinematic (zone_grizzly_hills.cpp:78-175)
-- PORTED 2026-11-13 (follow-up strip 2, unit A): ev6
-- emilyWaypointReached below, GetCurrentWaypointId switch (wilda
-- precedent) — wp9 Mrfloppy GUID latch, wp10 Talk(0) + SummonCreature
-- 26586, wp11 MoveFollow, wp17 MoveTo + Talk(2) + SummonCreature 26590
-- + SetFaction(35), wp18 MoveTo + DoCast 47184 on Mrfloppy, wp20
-- PerformEmote(34) (HandleEmoteCommand twin), wp21 Kill + ExitVehicle
-- + SetFaction(14) + MoveTo + Talk(6), wp22 RWORG DespawnOrUnsummon +
-- Emily MoveTo + Mrfloppy SetDeathState(ALIVE 0) + SetHealth(max) +
-- MoveFollow + Talk(7), wp24 GroupEventHappens(12027) + Talk(12,
-- player) + SetWalk(false), wp25 Talk(8), wp27 double
-- DespawnOrUnsummon (DisappearAndDie twin). Waypoint 19 EnterVehicle
-- is PORTED below on the creature:EnterVehicle bridge (live
-- 2026-11-13; fires once the deployment's Vehicle.dbc carries the
-- ravenous worg's VehicleId 35 kit row — TDB-verified). OnQuestAccept Talk
-- SAY_QUEST_ACCEPT 11 is PORTED below (event 31), including the
-- Mrfloppy MoveFollow + StartEscort + MoveWaypoint legs
-- (zone_grizzly_hills.cpp:187-196).
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
local SAY_WORGHAGGRO1 = 0 -- Um... I think one of those wolves is back...
local SAY_WORGHAGGRO2 = 1 -- He's going for Mr. Floppy!
local SAY_WORGRAGGRO3 = 2 -- Oh, no! Look, it's another wolf, and it's a biiiiiig one!
local SAY_WORGRAGGRO4 = 3 -- He's gonna eat Mr. Floppy! You gotta help Mr. Floppy! You just gotta!
local SAY_RANDOMAGGRO = 4 -- There's a big meanie attacking Mr. Floppy! Help!
local SAY_VICTORY2 = 6 -- Don't go toward the light, Mr. Floppy!
local SAY_VICTORY3 = 7 -- Mr. Floppy, you're ok! Thank you so much for saving Mr. Floppy!
local SAY_VICTORY4 = 8 -- I think I see the camp! We're almost home, Mr. Floppy! Let's go!
local SAY_QUEST_ACCEPT = 11 -- Are you ready, Mr. Floppy? Stay close to me and watch out for those wolves!
local SAY_QUEST_COMPLETE = 12 -- Thank you for helping me get back to the camp. Go tell Walter that I'm safe now!
local SPELL_MRFLOPPY = 47184 -- zone_grizzly_hills.cpp:43 (vehicle aura)
local FACTION_FRIENDLY = 35 -- SharedDefines.h:231
local FACTION_MONSTER = 14 -- SharedDefines.h:226
local EMOTE_ONESHOT_WOUND_CRITICAL = 34 -- SharedDefines.h:1967
local PET_FOLLOW_DIST = 1.0 -- PetDefines.h:84
local PET_FOLLOW_ANGLE = math.pi / 2 -- PetDefines.h:85 (M_PI/2)
local TEMPSUMMON_TIMED_OR_DEAD_DESPAWN = 1 -- ObjectDefines.h:59
local SUMMON_DESPAWN_MS = 120000 -- 2min (zone_grizzly_hills.cpp:94, :105)
local DEATHSTATE_ALIVE = 0 -- Unit.h:202
local WORG_SUMMON_ORIENTATION = 3.229 -- zone_grizzly_hills.cpp:94, :105

local QUEST_PERILOUS_ADVENTURE = 12027

-- Per-Emily GUID latches (C++ _mrfloppyGUID / _RavenousworgGUID,
-- zone_grizzly_hills.cpp:200-201), keyed by Emily's GUIDLow — the
-- legoso guid-keyed-table precedent. Resolved via GetCreatureByGUID
-- (lua_creature_events.go:4332); nil when the creature is gone, the
-- C++ ObjectAccessor::GetCreature null-guard twin.
local escortGUIDs = {}

local function emilyState(creature)
    local guid = creature:GetGUIDLow()
    local st = escortGUIDs[guid]
    if st == nil then
        st = {}
        escortGUIDs[guid] = st
    end
    return st
end

local function getMrfloppy(creature)
    local guid = emilyState(creature).mrfloppy
    if guid == nil then
        return nil
    end
    return creature:GetCreatureByGUID(guid)
end

local function getRavenousWorg(creature)
    local guid = emilyState(creature).rworg
    if guid == nil then
        return nil
    end
    return creature:GetCreatureByGUID(guid)
end

-- C++ JustEngagedWith: Talk(SAY_RANDOMAGGRO) (no target).
local function emilyEnterCombat(event, creature, target)
    creature:Talk(SAY_RANDOMAGGRO)
end

-- C++ OnQuestAccept (zone_grizzly_hills.cpp:187-196): quest 12027
-- -> Talk(SAY_QUEST_ACCEPT), Mrfloppy MoveFollow, EscortAI::Start.
-- C++ order: Talk, MoveFollow, Start (wilda Talk/SetFaction/
-- StartEscort/MoveWaypoint precedent for the StartEscort +
-- MoveWaypoint tail).
local function emilyQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_PERILOUS_ADVENTURE then
        return
    end
    creature:Talk(SAY_QUEST_ACCEPT)
    local mrfloppy = creature:FindNearestCreature(ENTRY_MRFLOPPY, 180.0)
    if mrfloppy ~= nil then
        mrfloppy:MoveFollow(creature, PET_FOLLOW_DIST, PET_FOLLOW_ANGLE)
    end
    creature:StartEscort(player)
    creature:MoveWaypoint()
end

RegisterCreatureEvent(ENTRY_EMILY, 1, emilyEnterCombat)
RegisterCreatureEvent(ENTRY_EMILY, 31, emilyQuestAccept)

-- C++ WaypointReached (zone_grizzly_hills.cpp:78-175): the waypoint id
-- is the DB point (GetCurrentWaypointId, wilda precedent); the player
-- nil-guard mirrors `if (!player) return` (:80-82). Arm order and
-- spell/entry IDs are C++-exact per leg; see the header for the
-- bridge file:line of each method used.
local function emilyWaypointReached(event, creature)
    local player = creature:GetEscortPlayer()
    if player == nil then
        return
    end
    local wp = creature:GetCurrentWaypointId()
    if wp == 9 then
        -- :86-89: latch the closest Mrfloppy within 100yd.
        local mrfloppy = creature:FindNearestCreature(ENTRY_MRFLOPPY, 100.0)
        if mrfloppy ~= nil then
            emilyState(creature).mrfloppy = mrfloppy:GetGUIDLow()
        end
    elseif wp == 10 then
        -- :90-96: Talk(0) + summon the hungry worg (feeds the latent
        -- ev19 emilyJustSummoned arm, already ported).
        local mrfloppy = getMrfloppy(creature)
        if mrfloppy ~= nil then
            creature:Talk(SAY_WORGHAGGRO1)
            creature:SummonCreature(NPC_HUNGRY_WORG, creature:GetX() + 5, creature:GetY() + 2, creature:GetZ() + 1, WORG_SUMMON_ORIENTATION, TEMPSUMMON_TIMED_OR_DEAD_DESPAWN, SUMMON_DESPAWN_MS)
        end
    elseif wp == 11 then
        -- :97-100: Mrfloppy follows Emily.
        local mrfloppy = getMrfloppy(creature)
        if mrfloppy ~= nil then
            mrfloppy:MoveFollow(creature, PET_FOLLOW_DIST, PET_FOLLOW_ANGLE)
        end
    elseif wp == 17 then
        -- :101-110: Mrfloppy MovePoint is Mrfloppy-gated; Talk(2) and
        -- the ravenous-worg summon are ungated in C++.
        local mrfloppy = getMrfloppy(creature)
        if mrfloppy ~= nil then
            mrfloppy:MoveTo(0, creature:GetX(), creature:GetY(), creature:GetZ())
        end
        creature:Talk(SAY_WORGRAGGRO3)
        local rworg = creature:SummonCreature(NPC_RAVENOUS_WORG, creature:GetX() + 10, creature:GetY() + 8, creature:GetZ() + 2, WORG_SUMMON_ORIENTATION, TEMPSUMMON_TIMED_OR_DEAD_DESPAWN, SUMMON_DESPAWN_MS)
        if rworg ~= nil then
            rworg:SetFaction(FACTION_FRIENDLY)
            emilyState(creature).rworg = rworg:GetGUIDLow()
        end
    elseif wp == 18 then
        -- :111-118: ravenous worg runs to Mrfloppy; Emily casts the
        -- vehicle aura (47184) on Mrfloppy.
        local mrfloppy = getMrfloppy(creature)
        if mrfloppy ~= nil then
            local rworg = getRavenousWorg(creature)
            if rworg ~= nil then
                rworg:MoveTo(0, mrfloppy:GetX(), mrfloppy:GetY(), mrfloppy:GetZ())
            end
            creature:CastSpell(mrfloppy, SPELL_MRFLOPPY)
        end
    elseif wp == 19 then
        -- :119-128: Mrfloppy boards the ravenous worg, gated on the
        -- vehicle aura (47184) — creature:EnterVehicle is the
        -- Unit::EnterVehicle bridge (lua_creature_events.go); the
        -- worg's kit comes from its template VehicleId 35
        -- (TDB-verified), so the leg no-ops until the deployment's
        -- Vehicle.dbc carries row 35.
        local mrfloppy = getMrfloppy(creature)
        if mrfloppy ~= nil then
            if mrfloppy:HasAura(SPELL_MRFLOPPY) then
                local rworg = getRavenousWorg(creature)
                if rworg ~= nil then
                    mrfloppy:EnterVehicle(rworg)
                end
            end
        end
    elseif wp == 20 then
        -- :129-132: ravenous worg plays the wound-critical emote
        -- (PerformEmote is the HandleEmoteCommand twin).
        local rworg = getRavenousWorg(creature)
        if rworg ~= nil then
            rworg:PerformEmote(EMOTE_ONESHOT_WOUND_CRITICAL)
        end
    elseif wp == 21 then
        -- :133-145: the ravenous worg kills Mrfloppy (C++
        -- Unit::Kill(RWORG, Mrfloppy), attacker first), Mrfloppy
        -- exits the vehicle, worg flips hostile and runs off; Talk(6).
        local mrfloppy = getMrfloppy(creature)
        if mrfloppy ~= nil then
            local rworg = getRavenousWorg(creature)
            if rworg ~= nil then
                mrfloppy:Kill(rworg)
                mrfloppy:ExitVehicle()
                rworg:SetFaction(FACTION_MONSTER)
                rworg:MoveTo(0, rworg:GetX() + 10, rworg:GetY() + 80, rworg:GetZ())
                creature:Talk(SAY_VICTORY2)
            end
        end
    elseif wp == 22 then
        -- :146-159: if Mrfloppy is dead, the worg disappears, Emily
        -- runs to Mrfloppy, Mrfloppy revives and follows; Talk(7).
        -- SetDeathState(ALIVE) alone leaves Health 0 in the Go model
        -- (IsDead is health-based; the ALIVE arm sets the field only),
        -- so SetHealth(max) rides along — the skyriss
        -- SetDeathState+SetHealth pairing precedent.
        local mrfloppy = getMrfloppy(creature)
        if mrfloppy ~= nil and mrfloppy:IsDead() then
            local rworg = getRavenousWorg(creature)
            if rworg ~= nil then
                rworg:DespawnOrUnsummon()
            end
            creature:MoveTo(0, mrfloppy:GetX(), mrfloppy:GetY(), mrfloppy:GetZ())
            mrfloppy:SetDeathState(DEATHSTATE_ALIVE)
            mrfloppy:SetHealth(mrfloppy:GetMaxHealth())
            mrfloppy:MoveFollow(creature, PET_FOLLOW_DIST, PET_FOLLOW_ANGLE)
            creature:Talk(SAY_VICTORY3)
        end
    elseif wp == 24 then
        -- :160-164: quest credit + Talk(12) to the player
        -- + walk mode off.
        player:GroupEventHappens(QUEST_PERILOUS_ADVENTURE, creature)
        creature:Talk(SAY_QUEST_COMPLETE, player)
        creature:SetWalk(false)
    elseif wp == 25 then
        -- :165-167: Talk(8).
        creature:Talk(SAY_VICTORY4)
    elseif wp == 27 then
        -- :168-172: both disappear (DisappearAndDie twin).
        creature:DespawnOrUnsummon()
        local mrfloppy = getMrfloppy(creature)
        if mrfloppy ~= nil then
            mrfloppy:DespawnOrUnsummon()
        end
    end
end

RegisterCreatureEvent(ENTRY_EMILY, 6, emilyWaypointReached)

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
