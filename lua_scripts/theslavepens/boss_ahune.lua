-- Ahune (The Slave Pens, Midsummer Fire Festival) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/TheSlavePens/
-- boss_ahune.cpp (boss_ahune — the AI class
-- AddSC_boss_ahune registers under "boss_ahune"). Fourth and
-- final script in The Slave Pens set per
-- outland_script_loader.cpp order (mennu, rokmar, quagmirran
-- closed); the_slave_pens.h EncounterCount is 3 — Ahune is
-- seasonal and instance-orchestrated.
-- Entry 25740 NPC_AHUNE (the_slave_pens.h) — verifiable from
-- the C++ sources: instance_the_slave_pens.cpp maps
-- { NPC_AHUNE, DATA_AHUNE } and { NPC_FROZEN_CORE,
-- DATA_FROZEN_CORE }; DATA_AHUNE = 4 is an instance-side
-- constant (unbridgeable). The creature_template ScriptName
-- bindings are DB-side (no TDB in this workspace). The file
-- has NO Talk calls on the boss itself (no Say enum lines are
-- ever called here) and no KilledUnit override — no event 3
-- registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent (per-GUID
-- scheduler pump); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention);
-- DoCastSelf takes nil as the target arm (felmyst convention).
-- Fight shape (C++-exact for all modeled arms): boss_ahune
-- (25740): OnEnterCombat(1): per-GUID reset + the C++
-- EVENT_INITIAL_EMERGE arm fires directly (the C++ schedules
-- it at 4ms — effectively instant on engage): DoCastSelf
-- SPELL_STAND 37752, DoCastSelf SPELL_AHUNE_SPANKY_HANDS
-- 46146, DoCastSelf SPELL_AHUNES_SHIELD 45954 (C++-exact arm
-- order) + 1s scheduler pump (a port of ExecuteEvent — 1s
-- granularity is exact for the 3s synch timer). BossAI base UpdateAI pre-loop (ScriptedCreature.cpp:578) + in-loop (:584) CASTING gates ported on the synch-health tick below (no boss UpdateAI override). Pump:
-- EVENT_SYNCH_HEALTH 46430 3s re-arm 3s — the fire arm is
-- ported in bossTick: FindNearestCreature(25865, 100.0)
-- approximates the instance->GetCreature(DATA_FROZEN_CORE)
-- lookup, and the 46430 script leg (hit health = caster
-- health) is modeled directly via SetHealth. The suicide
-- fallback when the core is absent has no equivalent.
-- OnDied(4): the cross-creature Unit::Kill of ahuneBunny/frozenCore
-- is PORTED below (tick 386 — FindNearestCreature + Kill are both
-- live, baltharus precedent; the old "cross-creature blocked" claim
-- is stale); the achievement DoCastSpellOnPlayers 62043, the LFG
-- FinishDungeon arm and the _JustDied arm are instance/cross-creature
-- blocked — the _JustDied leg is a C++ dead write anyway (no
-- SetBossNumber in instance_the_slave_pens.cpp).
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop
-- per-GUID state (the _Reset arm is instance-blocked). Melee
-- is engine-driven. The ctor's SetControlled(true, UNIT_STATE_ROOT)
-- is PORTED via creature:SetRooted on event 5 (spawn, which fires on
-- fresh spawn too, not only on respawn — creaturemotion.go:719-747 —
-- so the 2026-10-24 engage-time placement is retired to a redundant
-- re-assert below) + OnEnterCombat(1): the root is never cleared in
-- C++ (frozen-core event-5 ctor precedent).
-- EnterEvadeMode is PORTED below on ev2 (follow-up strip 2, unit D,
-- 2026-11-13): bunny DoAction(ACTION_STOP_EVENT) + summons.DespawnAll
-- (the frozen core) + self DespawnOrUnsummon, with a veto return so the
-- base evade never runs — the C++ override twin (boss_ahune.cpp:185-192).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (the DATA_AHUNE NOT_STARTED/IN_PROGRESS/DONE bookkeeping in
-- BossAI::JustEngagedWith instance bookkeeping / JustDied skipped
-- (base pulse + me-DoZone arms ported in ev1);
-- instance_the_slave_pens.cpp is PORTED 2026-11-10
-- (instance_the_slave_pens.lua: LoadObjectData entry latches +
-- flamecaller GUID capture via creature:SetGuidData, event 5);
-- the DoAction(ACTION_AHUNE_RETREAT) receiver (Submerge + the 35s
-- EVENT_EMERGE re-arm + Emerge's triggered RESURFACE 46402 self-cast and
-- its RemoveAurasDueToSpell(STAY_SUBMERGED/AHUNE_SELF_STUN) +
-- RemoveFlag(NOT_SELECTABLE) arms) is PORTED below (follow-up strip 2,
-- unit D, 2026-11-13: ahuneSubmerge/ahuneEmerge + ev38); the frozen-core
-- DoAction legs ride the core's own ev38 in npc_frozen_core.lua, and the
-- synch-health pump stops while submerged (events.Reset) and restarts on
-- Emerge while the core's phase-two pump runs the 46430 leg instead.
-- BossAI base CASTING gate PORTED on the synch-health tick (see pump gate
-- note; live creature:IsCasting bridge, lua_creature_events.go:1950); no
-- SpellScript bridge, and only one AuraScript bridge (RegisterAuraEvent,
-- AuraEventOnPeriodic = 1) — three of the nine script classes
-- in this file
-- (spell_ahune_synch_health, spell_ahune_spanky_hands,
-- spell_ahune_minion_despawner)
-- are documented only; spell_ice_spear_control_aura is PORTED
-- tick 189 below (RegisterAuraEvent 46371, iceSpearControlTick),
-- its delayer sibling spell_summon_ice_spear_delayer is PORTED
-- tick 191 below (RegisterAuraEvent 46878, iceSpearDelayerTick,
-- aura:GetTickNumber bridge), spell_summoning_rhyme_aura is
-- PORTED below (RegisterAuraEvent 45926, summoningRhymeTick), and
-- spell_ice_spear_target_picker / spell_slippery_floor_periodic /
-- spell_ice_bombardment_dest_picker are PORTED at the end of the
-- file (RegisterSpellEvent 46372 ev7+ev5, 46320 ev5, 46398 ev4);
-- npc_earthen_ring_flamecaller (25754) is
-- PORTED 2026-11-10 (npc_earthen_ring_flamecaller.lua: MovementInform
-- + SpellHit + DoAction + look-for-opening chain; its bunny-side
-- triggers are PORTED below, follow-up strip 1, unit C);
-- go_ahune_ice_stone (187882) is PORTED at the end of this file
-- (RegisterGameObjectGossipEvent select: bunny DoAction START +
-- Luma rhyme-aura cast via grid lookup, go:Despawn);
-- EMOTE_EARTHEN_ASSAULT 0, EMOTE_RETREAT 0, EMOTE_RESURFACE 1
-- and the Events/Actions/Phases/Points/Misc enums are referenced
-- only by the unregistered arms above (SAY_PLAYER_TEXT_1/2/3
-- (0/1/2) ride the ported 45926 below).

local SPELL_STAND = 37752
local SPELL_AHUNE_SPANKY_HANDS = 46146
local SPELL_AHUNES_SHIELD = 45954
local SPELL_SUBMERGED = 37751 -- boss_ahune.cpp:47
local SPELL_STAY_SUBMERGED = 46981 -- boss_ahune.cpp:41
local SPELL_AHUNE_SELF_STUN = 46416 -- boss_ahune.cpp:42
local UNIT_FIELD_FLAGS = 59 -- boss_curator.lua:37 precedent
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000 -- UnitDefines.h:149
local EMOTE_ONESHOT_SUBMERGE = 374 -- SharedDefines.h:2012
local ACTION_STOP_EVENT = -2574501 -- boss_ahune.cpp:129; top placement (not the
-- bunny const block below) so the ev2 EnterEvadeMode relay above can use it

local ENTRY_AHUNE = 25740
local ENTRY_AHUNE_BUNNY = 25745 -- NPC_AHUNE_LOC_BUNNY, the_slave_pens.h:53
local ENTRY_FROZEN_CORE = 25865

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetBoss(guid)
    cancelPump(guid)
    -- A pending EVENT_EMERGE one-shot dies with the boss (evade/death/
    -- re-engage): C++ EnterEvadeMode/JustDied never let it fire after.
    local st = states[guid]
    if st and st.emergeId then
        RemoveEventById(st.emergeId)
        st.emergeId = nil
    end
    states[guid] = nil
end

-- JustEngagedWith initial values: EVENT_SYNCH_HEALTH 3000.
local function freshState()
    return {
        synchHealth = 3000,
        submerged = false,
    }
end

-- Pump CASTING gate: the expired arm carries `and not
-- creature:IsCasting()` — while casting it fires nothing and
-- consumes no re-arm (else-branch countdown only, timer stays
-- expired); unexpired arms keep counting down, since C++
-- events.Update runs ahead of the gate (BossAI::UpdateAI
-- ScriptedCreature.cpp:571-589, no boss override).
local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- EVENT_SYNCH_HEALTH: 3s init, re-arm 3s — the fire arm
    -- casts 46430 on the frozen core (boss_ahune.cpp:234-236,
    -- 628-637: script sets the hit unit's health to the
    -- caster's health). With no SpellScript bridge the leg is
    -- modeled directly: FindNearestCreature(25865, 100.0)
    -- (illidan maiev precedent — the grid lookup approximates
    -- the C++ instance-GUID lookup) + SetHealth to Ahune's
    -- current health. The C++ suicide fallback when the core
    -- is absent has no equivalent; the reverse core->Ahune
    -- leg needs a pump in npc_frozen_core.lua, not this item.
    if st.synchHealth <= 1000 and not creature:IsCasting() then
        st.synchHealth = 3000
        local core = creature:FindNearestCreature(ENTRY_FROZEN_CORE, 100.0)
        if core then
            core:SetHealth(creature:GetHealth())
        end
    else
        st.synchHealth = st.synchHealth - 1000
    end
end

-- C++ ctor (boss_ahune.cpp:175) SetControlled(true, ROOT), never
-- cleared in C++ — event 5 (spawn) placement, frozen-core precedent.
RegisterCreatureEvent(ENTRY_AHUNE, 5, function(_, creature)
    creature:SetRooted(true)
end)

RegisterCreatureEvent(ENTRY_AHUNE, 1, function(_, creature)
    -- Re-assert the ctor root on engage (idempotent; the event-5
    -- handler above is the C++-exact placement).
    creature:SetRooted(true)
    local guid = creature:GetGUIDLow()
    resetBoss(guid)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    -- EVENT_INITIAL_EMERGE (C++-scheduled at 4ms — fires
    -- directly on engage): DoCastSelf(SPELL_STAND),
    -- DoCastSelf(SPELL_AHUNE_SPANKY_HANDS), DoCastSelf
    -- (SPELL_AHUNES_SHIELD), C++-exact arm order.
    creature:CastSpell(creature, SPELL_STAND)
    creature:CastSpell(creature, SPELL_AHUNE_SPANKY_HANDS)
    creature:CastSpell(creature, SPELL_AHUNES_SHIELD)
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_AHUNE, 2, function(_, creature)
    -- C++ boss_ahune::EnterEvadeMode (boss_ahune.cpp:185-192): bunny
    -- DoAction(ACTION_STOP_EVENT); summons.DespawnAll(); me->
    -- DespawnOrUnsummon(). PORTED follow-up strip 2, unit D. ev2 IS the
    -- EnterEvadeMode hook — the engine fires ev23 then ev2 from
    -- EnterEvadeMode when wasInCombat (creaturemotion.go:4718-4740) — so
    -- this handler is the override twin, not just a LeaveCombat cleanup.
    -- The bunny STOP relay re-arms the ice stone via the ported bunny
    -- ev38 (bunnyStopEvent below); the core is Ahune's summon
    -- (bunnySummonAhune), so summons.DespawnAll() = core:Despawn()
    -- (FindNearestCreature idiom, as in JustDied). Despawn is the
    -- DespawnOrUnsummon twin (lua_creature_events.go:3318). Returning
    -- true vetoes the base evade (creaturemotion.go:4734-4740) —
    -- C++-exact, since the override never runs the base evade (no threat
    -- clear / move home / heal-to-full / aura wipe); without the veto
    -- the base evade would run those on the despawned motion.
    local bunny = creature:FindNearestCreature(ENTRY_AHUNE_BUNNY, 100.0)
    if bunny then
        bunny:DoAction(ACTION_STOP_EVENT)
    end
    local core = creature:FindNearestCreature(ENTRY_FROZEN_CORE, 100.0)
    if core then
        core:Despawn()
    end
    resetBoss(creature:GetGUIDLow())
    creature:Despawn()
    return true
end)

RegisterCreatureEvent(ENTRY_AHUNE, 4, function(_, creature)
    -- C++ JustDied (boss_ahune.cpp:197-201): Unit::Kill(bunny) then
    -- Unit::Kill(frozenCore); bunny 25745 (NPC_AHUNE_LOC_BUNNY),
    -- core 25865. FindNearestCreature 100.0 is the in-file
    -- instance-lookup idiom (:158). Killing the core fires its
    -- (already ported) ev4 — loot-missile + minion-despawner casts —
    -- C++-exact cascade. Recursion-safe: Kill on an already-dead
    -- target is a Health == 0 no-op (boss_ai.go:9697-9700).
    local bunny = creature:FindNearestCreature(ENTRY_AHUNE_BUNNY, 100.0)
    if bunny then bunny:Kill(creature) end
    local core = creature:FindNearestCreature(ENTRY_FROZEN_CORE, 100.0)
    if core then core:Kill(creature) end
    resetBoss(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_AHUNE, 23, function(_, creature)
    resetBoss(creature:GetGUIDLow())
end)

-- C++ npc_ahune_bunny::JustSummoned (boss_ahune.cpp:350-357):
-- a summon whose entry is Ahune himself (25740, the
-- EVENT_SUMMON_AHUNE summon at :425) is skipped; every other
-- summon — in practice the three bonfire bunnies 25971-25973
-- from ACTION_START_EVENT (:370-372) — is zone-engaged.
-- summon:DoZoneInCombat() is the C++ DoZoneInCombat(summon)
-- twin (lua_creature_events.go:2255; npc_mellichar.lua:383 ev19
-- precedent); summon:GetEntry() rides lua_objects.go:278. The
-- _summons.Summon() leg (:356) is pure bookkeeping with no
-- observable effect at summon time — its only effects are
-- DespawnAll on JustDied (:359-362) and ACTION_STOP_EVENT
-- (:389), separate arms (ev4/ev38) outside this row, so
-- DoZoneInCombat alone is the faithful port of this arm. No
-- RegisterLuaBoss row is needed for 25745: Runtime.Load walks
-- every .lua file (runtime.go:899) and ev19 fires by entry
-- (fireCreatureSummoned, lua_creature_events.go:7880-7903).
-- Reachability note: the bunny's own summon arms (DoAction
-- ACTION_START_EVENT, EVENT_SUMMON_AHUNE) are PORTED below
-- (follow-up strip 1, unit C), so this hook is live — the same
-- receiver-ported/sender-later pattern as the flamecaller
-- SpellHit, whose sender cast is likewise ported below.
RegisterCreatureEvent(ENTRY_AHUNE_BUNNY, 19, function(_, creature, summon)
    if summon:GetEntry() == ENTRY_AHUNE then
        return
    end
    summon:DoZoneInCombat()
end)

-- C++ npc_ahune_bunny (boss_ahune.cpp:342-474) — PORTED follow-up strip 1,
-- unit C, 2026-11-13. The bunny is the Ahune event driver: the ice-stone
-- gossip sends ACTION_START_EVENT (see the go_ahune_ice_stone handler at
-- the end of this file), and the phase machine below drives the flamecaller
-- SHAMANS_LOOK_FOR_OPENING (46422) / FOUND_OPENING (46421) sender casts
-- feeding the already-ported flamecaller ev14 receiver
-- (npc_earthen_ring_flamecaller.lua:148). Entry 25745 NPC_AHUNE_LOC_BUNNY
-- (the_slave_pens.h:53); the instance guid store
-- (instance_the_slave_pens.lua) latches DATA_AHUNE=4 and
-- DATA_FLAMECALLER_000/001/002=7/8/9, read back via
-- creature:GetInstanceCreature (lua_creature_events.go:7081).
-- Phase machine: per-GUID state + one-shot CreateLuaEvent timers
-- (repeats default 1); each callback checks the phase it was scheduled
-- under (the EventMap phase gate) and drops if the phase flipped; every
-- phase flip cancels all pending timers first (the C++ _events.Reset()
-- on SetPhase). C++ UpdateAI has no CASTING/UpdateVictim gate on this AI
-- (it never enters combat itself), so the timers carry none.
-- _summons tracks ONLY the three bonfire bunnies: C++ JustSummoned
-- (:350-357, ported as ev19 above) skips Ahune himself, and the frozen
-- core is summoned by Ahune, not the bunny, so neither enters the
-- bunny's summon list; DespawnAll therefore despawns only the bonfires.
-- summonType numbers are the C++ TempSummonType values (Go twin,
-- summonTempCreature): MANUAL_DESPAWN 7, DEAD_DESPAWN 6, CORPSE_DESPAWN 4.
-- The bunny's summons resolve only with creature_template rows at runtime
-- (repo world.db is a 0-byte placeholder) — the same dormancy class as
-- every other SummonCreature port.

local ACTION_START_EVENT = -2574500 -- boss_ahune.cpp:128
local ACTION_AHUNE_RETREAT = -2586500 -- boss_ahune.cpp:132
local ACTION_AHUNE_RESURFACE = -2586501 -- boss_ahune.cpp:133
local ACTION_EMOTE_RESURFACE = -2575400 -- boss_ahune.cpp:134
local EMOTE_EARTHEN_ASSAULT = 0 -- boss_ahune.cpp:97
local PHASE_ONE = 0 -- boss_ahune.cpp:138
local PHASE_TWO = 1 -- boss_ahune.cpp:139
local DATA_AHUNE = 4 -- the_slave_pens.h:33
local DATA_FLAMECALLER_000 = 7 -- the_slave_pens.h:36
local NPC_SHAMAN_BONFIRE_BUNNY_000 = 25971 -- the_slave_pens.h:55
local NPC_SHAMAN_BONFIRE_BUNNY_001 = 25972 -- the_slave_pens.h:56
local NPC_SHAMAN_BONFIRE_BUNNY_002 = 25973 -- the_slave_pens.h:57
local SPELL_SUMMONING_VISUAL_1 = 45937 -- boss_ahune.cpp:61
local SPELL_SHAMANS_LOOK_FOR_OPENING = 46422 -- boss_ahune.cpp:66
local SPELL_FOUND_OPENING = 46421 -- boss_ahune.cpp:56
local SPELL_CLOSE_OPENING_VISUAL = 46236 -- boss_ahune.cpp:67
local SPELL_ICE_BOMBARD = 46397 -- boss_ahune.cpp:68
local SPELL_SUMMON_HAILSTONE = 46176 -- boss_ahune.cpp:60
local SPELL_SUMMON_COLDWEAVE = 46143 -- boss_ahune.cpp:58
local SPELL_SUMMON_FROSTWIND = 46382 -- boss_ahune.cpp:59
local SPELL_RESURFACE = 46402 -- boss_ahune.cpp:46
local GO_ICE_STONE = 187882 -- the_slave_pens.h:66

-- boss_ahune.cpp:155-161 SummonPositions (Ahune, frozen core, bonfires).
local BUNNY_SUMMON_POSITIONS = {
    { -99.1021, -233.7526, -1.22307, 1.588250 }, -- Ahune
    { -98.0151, -230.4555, -1.21089, 1.797689 }, -- Frozen core
    { -143.172, -147.6801, -3.16113, 4.852015 }, -- Bonfire bunny 000
    { -134.304, -145.7803, -1.70332, 4.677482 }, -- Bonfire bunny 001
    { -125.036, -144.2065, -1.91660, 4.991642 }, -- Bonfire bunny 002
}

-- boss_ahune.cpp:164-172 FlameCallerSpots.
local FLAMECALLER_SPOTS = {
    { -145.2233, -137.5543, -1.59056, 5.427049 },
    { -137.4383, -136.4050, -1.72384, 5.336747 },
    { -129.0413, -132.1494, -2.09285, 5.460842 },
}

local bunnyStates = {}

local function bunnyFreshState()
    return { phase = PHASE_ONE, submerged = false, timers = {}, summons = {} }
end

local function bunnyCancelTimers(guid)
    local st = bunnyStates[guid]
    if not st then
        return
    end
    for _, id in pairs(st.timers) do
        RemoveEventById(id)
    end
    st.timers = {}
end

-- One-shot schedule under a phase: the callback drops if the bunny has
-- left that phase since (EventMap phase gate); re-scheduling a key
-- cancels its pending timer first (vezax schedule idiom).
local function bunnySchedule(creature, guid, key, delayMs, phase, fn)
    local st = bunnyStates[guid]
    if not st then
        return
    end
    local old = st.timers[key]
    if old then
        RemoveEventById(old)
    end
    st.timers[key] = CreateLuaEvent(function()
        st.timers[key] = nil
        local cur = bunnyStates[guid]
        if not cur or cur.phase ~= phase then
            return
        end
        fn(creature, guid)
    end, delayMs)
end

-- C++ ResetFlameCallers (boss_ahune.cpp:462-467): each flamecaller
-- EnterEvadeMode — the evade fires the flamecaller's ev23 Reset, which
-- cancels its look-for-opening chain (npc_earthen_ring_flamecaller.lua).
local function bunnyResetFlameCallers(creature)
    for counter = 0, 2 do
        local flamecaller = creature:GetInstanceCreature(DATA_FLAMECALLER_000 + counter)
        if flamecaller then
            flamecaller:EnterEvadeMode()
        end
    end
end

-- C++ _summons.DespawnAll() (:359-362, :389): despawn the tracked
-- bonfire bunnies. Despawn() on an already-gone motion is a no-op
-- (lua_creature_events.go:3305), so no alive guard is needed.
local function bunnyDespawnSummons(guid)
    local st = bunnyStates[guid]
    if not st then
        return
    end
    for _, summon in ipairs(st.summons) do
        summon:Despawn()
    end
    st.summons = {}
end

-- C++ EVENT_START_LOOKING_FOR_OPENING (:405-410): Talk(EMOTE_EARTHEN_
-- ASSAULT) + DoCast(flamecaller, SPELL_SHAMANS_LOOK_FOR_OPENING, true)
-- per flamecaller — the sender cast for the flamecaller ev14 receiver.
local function bunnyStartLooking(creature, guid)
    creature:Talk(EMOTE_EARTHEN_ASSAULT)
    for counter = 0, 2 do
        local flamecaller = creature:GetInstanceCreature(DATA_FLAMECALLER_000 + counter)
        if flamecaller then
            creature:CastSpell(flamecaller, SPELL_SHAMANS_LOOK_FOR_OPENING, true)
        end
    end
end

-- C++ EVENT_SUMMON_HAILSTONE (:411-413): DoCast(SPELL_SUMMON_HAILSTONE),
-- one-shot (re-armed only by phase-one entry).
local function bunnySummonHailstone(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMON_HAILSTONE)
end

-- C++ EVENT_SUMMON_FROSTWIND (:421-423): DoCast(SPELL_SUMMON_FROSTWIND).
local function bunnySummonFrostwind(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMON_FROSTWIND)
end

-- C++ EVENT_SUMMON_COLDWEAVE (:414-420): DoCast(SPELL_SUMMON_COLDWEAVE)
-- twice + Repeat(8s); while _submerged, schedule EVENT_SUMMON_FROSTWIND
-- in 4s (PHASE_ONE).
local function bunnySummonColdweave(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMON_COLDWEAVE)
    creature:CastSpell(creature, SPELL_SUMMON_COLDWEAVE)
    local st = bunnyStates[guid]
    if not st then
        return
    end
    bunnySchedule(creature, guid, "coldweave", 8000, PHASE_ONE, bunnySummonColdweave)
    if st.submerged then
        bunnySchedule(creature, guid, "frostwind", 4000, PHASE_ONE, bunnySummonFrostwind)
    end
end

-- C++ EVENT_SUMMON_AHUNE (:424-431): SummonCreature(NPC_AHUNE,
-- TEMPSUMMON_DEAD_DESPAWN); the summoned Ahune summons the frozen core
-- (TEMPSUMMON_CORPSE_DESPAWN) + DoZoneInCombat(ahune) + DoCast(ahune,
-- SPELL_RESURFACE). Neither Ahune nor the core enters the bunny's
-- _summons list (see header note above).
local function bunnySummonAhune(creature, guid)
    local pos = BUNNY_SUMMON_POSITIONS[1]
    local ahune = creature:SummonCreature(ENTRY_AHUNE, pos[1], pos[2], pos[3], pos[4], 6)
    if not ahune then
        return
    end
    local corePos = BUNNY_SUMMON_POSITIONS[2]
    ahune:SummonCreature(ENTRY_FROZEN_CORE, corePos[1], corePos[2], corePos[3], corePos[4], 4)
    ahune:DoZoneInCombat()
    creature:CastSpell(ahune, SPELL_RESURFACE)
end

-- C++ EVENT_CLOSE_OPENING (:432-436): flamecaller[0] DoAction
-- (ACTION_EMOTE_RESURFACE) + DoCast(SPELL_CLOSE_OPENING_VISUAL) +
-- DoCastSelf(SPELL_ICE_BOMBARD).
local function bunnyCloseOpening(creature, guid)
    local flamecaller = creature:GetInstanceCreature(DATA_FLAMECALLER_000)
    if flamecaller then
        flamecaller:DoAction(ACTION_EMOTE_RESURFACE)
    end
    creature:CastSpell(creature, SPELL_CLOSE_OPENING_VISUAL)
    creature:CastSpell(creature, SPELL_ICE_BOMBARD)
end

-- Phase flips reference each other: forward-declare with plain
-- assignments below (never re-`local function` after a forward declare).
local bunnyPhaseOne, bunnyPhaseTwo

-- C++ EVENT_AHUNE_PHASE_ONE (:447-455): _submerged = true; Reset;
-- SetPhase(PHASE_ONE); schedule COLDWEAVE 8s / HAILSTONE 5s /
-- START_LOOKING 5s / PHASE_TWO 100s (all PHASE_ONE).
bunnyPhaseOne = function(creature, guid)
    local st = bunnyStates[guid]
    if not st then
        return
    end
    st.submerged = true
    bunnyCancelTimers(guid)
    st.phase = PHASE_ONE
    bunnySchedule(creature, guid, "coldweave", 8000, PHASE_ONE, bunnySummonColdweave)
    bunnySchedule(creature, guid, "hailstone", 5000, PHASE_ONE, bunnySummonHailstone)
    bunnySchedule(creature, guid, "start_looking", 5000, PHASE_ONE, bunnyStartLooking)
    bunnySchedule(creature, guid, "phase_two", 100000, PHASE_ONE, bunnyPhaseTwo)
end

-- C++ EVENT_AHUNE_PHASE_TWO (:437-445): DoCast(flamecaller[0],
-- SPELL_FOUND_OPENING) — the sender cast for the flamecaller ev14
-- EMOTE_RETREAT leg — + Ahune DoAction(ACTION_AHUNE_RETREAT); Reset;
-- SetPhase(PHASE_TWO); schedule CLOSE_OPENING 25s / PHASE_ONE 35s
-- (both PHASE_TWO). The retreat receiver (Ahune Submerge/Emerge) is
-- PORTED below (ahuneSubmerge/ahuneEmerge + ev38, follow-up strip 2,
-- unit D), so this DoAction send now lands.
bunnyPhaseTwo = function(creature, guid)
    local flamecaller = creature:GetInstanceCreature(DATA_FLAMECALLER_000)
    if flamecaller then
        creature:CastSpell(flamecaller, SPELL_FOUND_OPENING)
    end
    local ahune = creature:GetInstanceCreature(DATA_AHUNE)
    if ahune then
        ahune:DoAction(ACTION_AHUNE_RETREAT)
    end
    bunnyCancelTimers(guid)
    local st = bunnyStates[guid]
    if st then
        st.phase = PHASE_TWO
    end
    bunnySchedule(creature, guid, "close_opening", 25000, PHASE_TWO, bunnyCloseOpening)
    bunnySchedule(creature, guid, "phase_one", 35000, PHASE_TWO, bunnyPhaseOne)
end

-- C++ DoAction(ACTION_START_EVENT) (:367-386): DoCastSelf
-- (SPELL_SUMMONING_VISUAL_1); summon the three bonfire bunnies
-- (TEMPSUMMON_MANUAL_DESPAWN); MovePoint each flamecaller to its
-- FlameCallerSpot; _submerged = false; Reset; SetPhase(PHASE_ONE);
-- schedule SUMMON_AHUNE 10s / START_LOOKING 14s / COLDWEAVE 22s /
-- HAILSTONE 14s / PHASE_TWO 108s (all PHASE_ONE).
local function bunnyStartEvent(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMONING_VISUAL_1)
    local st = bunnyStates[guid]
    if not st then
        return
    end
    local bonfireEntries = {
        NPC_SHAMAN_BONFIRE_BUNNY_000,
        NPC_SHAMAN_BONFIRE_BUNNY_001,
        NPC_SHAMAN_BONFIRE_BUNNY_002,
    }
    for counter = 0, 2 do
        local pos = BUNNY_SUMMON_POSITIONS[3 + counter]
        local bonfire = creature:SummonCreature(bonfireEntries[counter + 1],
            pos[1], pos[2], pos[3], pos[4], 7)
        if bonfire then
            table.insert(st.summons, bonfire)
        end
    end
    for counter = 0, 2 do
        local flamecaller = creature:GetInstanceCreature(DATA_FLAMECALLER_000 + counter)
        if flamecaller then
            local spot = FLAMECALLER_SPOTS[counter + 1]
            flamecaller:MoveTo(counter, spot[1], spot[2], spot[3])
        end
    end
    st.submerged = false
    bunnyCancelTimers(guid)
    st.phase = PHASE_ONE
    bunnySchedule(creature, guid, "summon_ahune", 10000, PHASE_ONE, bunnySummonAhune)
    bunnySchedule(creature, guid, "start_looking", 14000, PHASE_ONE, bunnyStartLooking)
    bunnySchedule(creature, guid, "coldweave", 22000, PHASE_ONE, bunnySummonColdweave)
    bunnySchedule(creature, guid, "hailstone", 14000, PHASE_ONE, bunnySummonHailstone)
    bunnySchedule(creature, guid, "phase_two", 108000, PHASE_ONE, bunnyPhaseTwo)
end

-- C++ DoAction(ACTION_STOP_EVENT) (:387-396): _summons.DespawnAll();
-- _events.Reset(); ResetFlameCallers(); SummonGameObject(GO_ICE_STONE,
-- 0s). The GO re-summon is template-gated at runtime like SummonCreature;
-- the quaternion (0,0,0.9366722,0.3502074) encodes the same yaw passed
-- as o=2.426008, so the Go (entry,x,y,z,o[,respawnSecs]) call is exact.
local function bunnyStopEvent(creature, guid)
    bunnyDespawnSummons(guid)
    bunnyCancelTimers(guid)
    bunnyResetFlameCallers(creature)
    creature:SummonGameObject(GO_ICE_STONE, -69.90455, -162.2449, -2.366563, 2.426008, 0)
end

-- C++ npc_ahune_bunny::DoAction (:364-366 / :387-396).
RegisterCreatureEvent(ENTRY_AHUNE_BUNNY, 38, function(_, creature, action)
    local guid = creature:GetGUIDLow()
    if not bunnyStates[guid] then
        bunnyStates[guid] = bunnyFreshState()
    end
    if action == ACTION_START_EVENT then
        bunnyStartEvent(creature, guid)
    elseif action == ACTION_STOP_EVENT then
        bunnyStopEvent(creature, guid)
    end
end)

-- C++ npc_ahune_bunny::JustDied (:358-362): _summons.DespawnAll() +
-- ResetFlameCallers(); timers and state are dropped with the bunny.
RegisterCreatureEvent(ENTRY_AHUNE_BUNNY, 4, function(_, creature)
    local guid = creature:GetGUIDLow()
    bunnyDespawnSummons(guid)
    bunnyCancelTimers(guid)
    bunnyResetFlameCallers(creature)
    bunnyStates[guid] = nil
end)

-- C++ boss_ahune::Submerge (boss_ahune.cpp:260-270): frozen core
-- DoAction(ACTION_AHUNE_RETREAT); RemoveAurasDueToSpell(AHUNES_SHIELD);
-- DoCastSelf(SUBMERGED, true); DoCastSelf(SELF_STUN, true);
-- DoCastSelf(STAY_SUBMERGED, true); HandleEmoteCommand(SUBMERGE);
-- events.Reset(). PORTED follow-up strip 2, unit D. The core send uses
-- the file's FindNearestCreature idiom (bossTick/JustDied); the core's
-- own ev38 receiver lives in npc_frozen_core.lua. RemoveAura is the
-- RemoveAurasDueToSpell twin (lua_creature_events.go:1761 — one aura per
-- spell per unit in the Go store). PerformEmote is the
-- HandleEmoteCommand twin (lua_creature_events.go:4488).
-- events.Reset() = cancelPump: the synch-health pump stops while
-- submerged, and the core's phase-two synch pump (npc_frozen_core.lua)
-- runs the 46430 leg instead. C++-exact arm order.
local function ahuneSubmerge(creature, guid)
    local st = states[guid]
    if not st then
        return
    end
    local core = creature:FindNearestCreature(ENTRY_FROZEN_CORE, 100.0)
    if core then
        core:DoAction(ACTION_AHUNE_RETREAT)
    end
    creature:RemoveAura(SPELL_AHUNES_SHIELD)
    creature:CastSpell(creature, SPELL_SUBMERGED, true)
    creature:CastSpell(creature, SPELL_AHUNE_SELF_STUN, true)
    creature:CastSpell(creature, SPELL_STAY_SUBMERGED, true)
    creature:PerformEmote(EMOTE_ONESHOT_SUBMERGE)
    cancelPump(guid)
    st.submerged = true
end

-- C++ boss_ahune::Emerge (boss_ahune.cpp:246-258): frozen core
-- DoAction(ACTION_AHUNE_RESURFACE); DoCastSelf(AHUNES_SHIELD);
-- RemoveAurasDueToSpell(SELF_STUN); RemoveAurasDueToSpell(STAY_SUBMERGED);
-- DoCastSelf(STAND); DoCastSelf(RESURFACE, true);
-- RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE);
-- ScheduleEvent(EVENT_SYNCH_HEALTH, 3s). PORTED follow-up strip 2,
-- unit D. RemoveFlag is bridged on the creature event surface
-- (lua_creature_events.go:6489, field 59); the synch-health re-arm
-- restarts the 1s pump with a fresh 3s countdown (freshState + bossTick,
-- the ev1 idiom). The submerged guard makes a stray/duplicate emerge a
-- no-op; the normal path always schedules it from a submerged state.
-- C++-exact arm order.
local function ahuneEmerge(creature, guid)
    local st = states[guid]
    if not st or not st.submerged then
        return
    end
    local core = creature:FindNearestCreature(ENTRY_FROZEN_CORE, 100.0)
    if core then
        core:DoAction(ACTION_AHUNE_RESURFACE)
    end
    creature:CastSpell(creature, SPELL_AHUNES_SHIELD)
    creature:RemoveAura(SPELL_AHUNE_SELF_STUN)
    creature:RemoveAura(SPELL_STAY_SUBMERGED)
    creature:CastSpell(creature, SPELL_STAND)
    creature:CastSpell(creature, SPELL_RESURFACE, true)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    st.submerged = false
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end

-- C++ boss_ahune::DoAction (boss_ahune.cpp:213-220): ACTION_AHUNE_RETREAT
-- → Submerge() + ScheduleEvent(EVENT_EMERGE, 35s). PORTED follow-up
-- strip 2, unit D. Sender: the bunny phase machine
-- (EVENT_AHUNE_PHASE_TWO, ported above) — ev38 is the
-- CreatureAI::DoAction dispatch twin (lua_creature_events.go:837-859),
-- so the send lands here. The 35s emerge is scheduled AFTER Submerge()
-- returns, so Submerge's events.Reset() (cancelPump) cannot eat it; a
-- pending emerge is cancelled first on re-schedule (vezax idiom — C++
-- would double-schedule, this is the safe equivalent).
RegisterCreatureEvent(ENTRY_AHUNE, 38, function(_, creature, action)
    if action ~= ACTION_AHUNE_RETREAT then
        return
    end
    local guid = creature:GetGUIDLow()
    local st = states[guid]
    if not st then
        states[guid] = freshState()
        st = states[guid]
    end
    ahuneSubmerge(creature, guid)
    if st.emergeId then
        RemoveEventById(st.emergeId)
    end
    st.emergeId = CreateLuaEvent(function()
        st.emergeId = nil
        ahuneEmerge(creature, guid)
    end, 35000)
end)

-- C++ spell_ice_spear_control_aura::PeriodicTick
-- (boss_ahune.cpp:745-749): on each tick of the ice spear control
-- aura (46371, self-cast by Ahune on engage — SPELL_ICE_SPEAR_AURA,
-- boss_ahune.cpp:286), the caster casts the ice spear target picker
-- (46372) on itself, non-triggered (no `true` arg in C++). The
-- caster is reached via aura:GetCaster() (bridged, lua_aura.go).
-- No PreventDefaultAction in C++, so the handler returns nothing.
-- PORTED tick 189 via RegisterAuraEvent (46371, AuraEventOnPeriodic
-- = 1).
local SPELL_ICE_SPEAR_AURA = 46371 -- boss_ahune.cpp:76
local SPELL_ICE_SPEAR_TARGET_PICKER = 46372 -- boss_ahune.cpp:77
local SPELL_SUMMON_ICE_SPEAR_DELAYER = 46878 -- boss_ahune.cpp:687
local SPELL_SUMMON_ICE_SPEAR_GO = 46369 -- boss_ahune.cpp:75
local SPELL_ICE_SPEAR_KNOCKBACK = 46360 -- boss_ahune.cpp:74
local GO_ICE_SPEAR = 188077 -- the_slave_pens.h:65

local function iceSpearControlTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster then
        caster:CastSpell(caster, SPELL_ICE_SPEAR_TARGET_PICKER)
    end
end

RegisterAuraEvent(SPELL_ICE_SPEAR_AURA, 1, iceSpearControlTick)

-- C++ spell_summon_ice_spear_delayer::PeriodicTick
-- (boss_ahune.cpp:697-719): the caster is reached via
-- aura:GetCaster(); tick 1: non-triggered self-cast
-- SPELL_SUMMON_ICE_SPEAR_GO (46369); tick 3: the nearest ice spear
-- GO (188077) within 3y is used (UseDoorOrButton) and the caster
-- DoCastAOEs SPELL_ICE_SPEAR_KNOCKBACK (46360) triggered (the
-- self-cast twin, selin/mr_smite precedent); tick 5: the nearest
-- ice spear GO is deleted and the caster despawns. The GO Delete
-- has no bridge, so the port uses the GO Despawn twin (hide in
-- instance, lua_objects.go) — documented delta. No
-- PreventDefaultAction in C++, so the handler returns nothing.
-- PORTED tick 191 via RegisterAuraEvent (46878, AuraEventOnPeriodic
-- = 1) on the new aura:GetTickNumber bridge.
local function iceSpearDelayerTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    local tick = aura:GetTickNumber()
    if tick == 1 then
        caster:CastSpell(caster, SPELL_SUMMON_ICE_SPEAR_GO)
    elseif tick == 3 then
        local spike = caster:FindNearestGameObject(GO_ICE_SPEAR, 3.0)
        if spike ~= nil then
            spike:UseDoorOrButton()
        end
        caster:CastSpell(caster, SPELL_ICE_SPEAR_KNOCKBACK, true)
    elseif tick == 5 then
        local spike = caster:FindNearestGameObject(GO_ICE_SPEAR, 3.0)
        if spike ~= nil then
            spike:Despawn()
        end
        caster:Despawn()
    end
end

RegisterAuraEvent(SPELL_SUMMON_ICE_SPEAR_DELAYER, 1, iceSpearDelayerTick)

-- C++ spell_summoning_rhyme_aura::PeriodicTick
-- (boss_ahune.cpp:644-685): the aura is cast on the player by Luma
-- Skymother (25697, the_slave_pens.h:52) from the ice-stone gossip;
-- every tick the player self-casts SPELL_FORCE_WHISP_FLIGHT (46603),
-- non-triggered in C++ (the Go player funnel is triggered-only,
-- castSpellDirectWithOverrides — documented delta); tick 1: Luma
-- says SAY_PLAYER_TEXT_1 (group 0, creature_text 25697) to the
-- player and the player self-casts SPELL_SUMMONING_RHYME_BONFIRE
-- (45930) triggered; tick 2: SAY_PLAYER_TEXT_2 (group 1); tick 3:
-- SAY_PLAYER_TEXT_3 (group 2) and the aura is removed. The
-- GetCaster()/GetTarget() nil + type gates are the C++ ToCreature/
-- ToPlayer null checks. Binding 45926 (AC
-- 'spell_summoning_rhyme_aura'; TDB 26091 renames it
-- 'spell_ahune_summoning_rhyme_aura', a name this TC tree never
-- registers). PORTED via RegisterAuraEvent (45926,
-- AuraEventOnPeriodic = 1); aura:GetTickNumber reads 1 on the
-- first tick, like C++ (lua_aura.go).
local SPELL_SUMMONING_RHYME_AURA = 45926 -- boss_ahune.cpp:644
local SPELL_SUMMONING_RHYME_BONFIRE = 45930 -- boss_ahune.cpp:64
local SPELL_FORCE_WHISP_FLIGHT = 46603 -- boss_ahune.cpp:65
local SAY_PLAYER_TEXT_1 = 0 -- boss_ahune.cpp:104
local SAY_PLAYER_TEXT_2 = 1 -- boss_ahune.cpp:105
local SAY_PLAYER_TEXT_3 = 2 -- boss_ahune.cpp:106
local TYPEID_UNIT = 3
local TYPEID_PLAYER = 4

local function summoningRhymeTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or target == nil then
        return
    end
    if caster:GetTypeId() ~= TYPEID_UNIT or target:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    target:CastSpell(target, SPELL_FORCE_WHISP_FLIGHT)
    local tick = aura:GetTickNumber()
    if tick == 1 then
        caster:Talk(SAY_PLAYER_TEXT_1, target)
        target:CastSpell(target, SPELL_SUMMONING_RHYME_BONFIRE, true)
    elseif tick == 2 then
        caster:Talk(SAY_PLAYER_TEXT_2, target)
    elseif tick == 3 then
        caster:Talk(SAY_PLAYER_TEXT_3, target)
        aura:Remove()
    end
end

RegisterAuraEvent(SPELL_SUMMONING_RHYME_AURA, 1, summoningRhymeTick)

-- C++ spell_ice_spear_target_picker (boss_ahune.cpp:749-779):
-- FilterTargets keeps one random target from the EFFECT_0
-- TARGET_UNIT_SRC_AREA_ENTRY list; HandleDummy then has the caster
-- cast SPELL_SUMMON_ICE_SPEAR_BUNNY (46359) on that hit unit,
-- triggered. PORTED via RegisterSpellEvent (46372,
-- OnObjectAreaTargetSelect = 7 + OnEffectHitTarget = 5); the ev7
-- handler (event, spell, effectIndex, targets) picks via
-- targets:GetTargets()/Clear()/Add (lua_spell.go target-list
-- object) and the ev5 handler casts on the surviving hit unit.
-- The picker is cast by the creature control-aura tick above, so
-- both hooks ride the creature-cast path (boss_ai.go:3151/3373).
-- Bindings 46372 (AC 'spell_ice_spear_target_picker'; TDB 26091
-- renames it 'spell_ahune_ice_spear_target_picker').
local SPELL_SUMMON_ICE_SPEAR_BUNNY = 46359 -- boss_ahune.cpp:73

local function iceSpearTargetPickerFilter(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    if targets:GetCount() == 0 then
        return
    end
    local all = targets:GetTargets()
    local picked = all[math.random(#all)]
    if picked == nil then
        return
    end
    targets:Clear()
    targets:Add(picked)
end

local function iceSpearTargetPickerHit(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(target, SPELL_SUMMON_ICE_SPEAR_BUNNY, true)
end

RegisterSpellEvent(SPELL_ICE_SPEAR_TARGET_PICKER, 7, iceSpearTargetPickerFilter)
RegisterSpellEvent(SPELL_ICE_SPEAR_TARGET_PICKER, 5, iceSpearTargetPickerHit)

-- C++ spell_slippery_floor_periodic::HandleScriptEffect
-- (boss_ahune.cpp:781-801): EFFECT_0 SCRIPT_EFFECT — a moving hit
-- unit self-casts SPELL_SLIPPERY_FLOOR_SLIP (45947) and
-- SPELL_SLIPPERY_FLOOR_YOU_SLIPPED (45946), both triggered; a
-- stationary hit unit gets nothing. PORTED via RegisterSpellEvent
-- (46320, OnEffectHitTarget = 5); IsMoving is bridged on both
-- surfaces (lua_player.go:2028, lua_creature_events.go:493).
-- Bindings 46320 (AC 'spell_slippery_floor_periodic'; TDB 26091
-- renames it 'spell_ahune_slippery_floor_periodic').
local SPELL_SLIPPERY_FLOOR_PERIODIC = 46320 -- boss_ahune.cpp:83
local SPELL_SLIPPERY_FLOOR_SLIP = 45947 -- boss_ahune.cpp:84
local SPELL_SLIPPERY_FLOOR_YOU_SLIPPED = 45946 -- boss_ahune.cpp:85

local function slipperyFloorHit(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    if target:IsMoving() then
        target:CastSpell(target, SPELL_SLIPPERY_FLOOR_SLIP, true)
        target:CastSpell(target, SPELL_SLIPPERY_FLOOR_YOU_SLIPPED, true)
    end
end

RegisterSpellEvent(SPELL_SLIPPERY_FLOOR_PERIODIC, 5, slipperyFloorHit)

-- C++ spell_ice_bombardment_dest_picker::HandleScriptEffect
-- (boss_ahune.cpp:846-867): EFFECT_0 DUMMY — the caster casts
-- SPELL_ICE_BOMBARDMENT (46396) triggered at the hit destination.
-- PORTED via RegisterSpellEvent (46398, OnEffectHit = 4); the dest
-- reads via spell:GetExplTargetDest() (lua_spell.go:501, three
-- values or three nils) and the cast lands via creature
-- CastSpellAoF with the triggered arg (lua_creature_events.go:1052).
-- Bindings 46398 (AC 'spell_ice_bombardment_dest_picker'; TDB
-- 26091 renames it 'spell_ahune_ice_bombardment_dest_picker').
local SPELL_ICE_BOMBARDMENT_DEST_PICKER = 46398 -- boss_ahune.cpp:69
local SPELL_ICE_BOMBARDMENT = 46396 -- boss_ahune.cpp:70

local function iceBombardmentDestPickerHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local destX, destY, destZ = spell:GetExplTargetDest()
    if destX == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpellAoF(destX, destY, destZ, SPELL_ICE_BOMBARDMENT, true)
end

RegisterSpellEvent(SPELL_ICE_BOMBARDMENT_DEST_PICKER, 4, iceBombardmentDestPickerHit)

-- C++ go_ahune_ice_stone::OnGossipSelect (boss_ahune.cpp:589-610) —
-- PORTED follow-up strip 1, unit C, 2026-11-13: ClearGossipMenuFor +
-- bunny DoAction(ACTION_START_EVENT) + Luma CastSpell(player,
-- SPELL_SUMMONING_RHYME_AURA 45926, true) + CloseGossipMenuFor +
-- me->Delete(). RegisterGameObjectGossipEvent select (event 2,
-- GossipEventOnSelect, engine/scripting/runtime.go:158) with args
-- (event, player, go, sender, action, code, listID)
-- (gameobject_hooks.go:226). The instance lookups
-- (_instance->GetCreature(DATA_AHUNE_BUNNY/DATA_LUMA_SKYMOTHER)) have
-- no GO-side guid-store reader, so the grid lookup approximates them
-- (go:FindNearestCreature, 100.0 — the same idiom as this file's
-- JustDied leg); go:Despawn() is the me->Delete() twin
-- (sapphiron-birth precedent). Returns true (handled — C++ returns
-- true, vetoing the native select arms).
local NPC_LUMA_SKYMOTHER = 25697 -- the_slave_pens.h:52

local function iceStoneGossipSelect(_, player, go, sender, action, code, listId)
    if player == nil then
        return true
    end
    player:GossipClearMenu()
    local bunny = go:FindNearestCreature(ENTRY_AHUNE_BUNNY, 100.0)
    if bunny then
        bunny:DoAction(ACTION_START_EVENT)
    end
    local luma = go:FindNearestCreature(NPC_LUMA_SKYMOTHER, 100.0)
    if luma then
        luma:CastSpell(player, SPELL_SUMMONING_RHYME_AURA, true)
    end
    player:GossipComplete()
    go:Despawn()
    return true
end

RegisterGameObjectGossipEvent(GO_ICE_STONE, 2, iceStoneGossipSelect)
