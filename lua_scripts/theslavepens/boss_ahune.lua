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
-- granularity is exact for the 3s synch timer). Pump:
-- EVENT_SYNCH_HEALTH 46430 3s re-arm 3s — timer bookkeeping
-- only: the whole fire arm is instance->GetCreature(DATA_
-- FROZEN_CORE) cross-creature gated (DoCast(frozenCore,
-- SPELL_SYNCH_HEALTH, true), else DoCastSelf(SPELL_SUICIDE)),
-- and no cross-creature bridge exists (kalithresh rage
-- precedent). OnDied(4): cleanup (the achievement
-- DoCastSpellOnPlayers 62043, the cross-creature Unit::Kill
-- of ahuneBunny/frozenCore, the LFG FinishDungeon arm and the
-- _JustDied arm are instance/cross-creature blocked).
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop
-- per-GUID state (the _Reset arm is instance-blocked). Melee
-- is engine-driven. The ctor's SetControlled(true,
-- UNIT_STATE_ROOT) has no unit-state bridge (mennu/thespia
-- precedent). EnterEvadeMode is instance/cross-creature/
-- summon gated (ahuneBunny DoAction STOP_EVENT,
-- summons.DespawnAll, DespawnOrUnsummon) — unmodeled.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (the DATA_AHUNE NOT_STARTED/IN_PROGRESS/DONE bookkeeping in
-- BossAI::JustEngagedWith/JustDied skipped;
-- instance_the_slave_pens.cpp is a placeholder and stays
-- blocked on the instance-script model); no cross-creature
-- bridge — the DoAction(ACTION_AHUNE_RETREAT) entry to
-- Submerge() is unreachable: it is fired only cross-creature
-- by npc_ahune_bunny's EVENT_AHUNE_PHASE_TWO, and the whole
-- bunny phase machine (ACTION_START_EVENT from the
-- go_ahune_ice_stone gossip, EVENT_SUMMON_AHUNE/FROZEN_CORE
-- summons, EVENT_SUMMON_HAILSTONE/COLDWEAVE/FROSTWIND casts,
-- EVENT_START_LOOKING_FOR_OPENING + the cross-creature
-- SHAMANS_LOOK_FOR_OPENING/FOUND_OPENING relay, EVENT_CLOSE_
-- OPENING/EVENT_AHUNE_PHASE_ONE/TWO phase flips with the
-- DATA_FLAMECALLER_*/DATA_BONFIRE_BUNNY_*/DATA_BEAM_BUNNY_*
-- guid lookups and MovePoint choreography, SummonPositions/
-- FlameCallerSpots coords) is summon/instance/GO/movement
-- driven and stays unregistered (naga-distiller empty-skeleton
-- precedent) — so the retreat/emerge machine (Submerge, the
-- EVENT_EMERGE 35s re-arm, Emerge's triggered RESURFACE 46402
-- self-cast and its RemoveAurasDueToSpell(STAY_SUBMERGED/
-- AHUNE_SELF_STUN) + RemoveFlag(NOT_SELECTABLE) arms) is
-- documented only; no flag bridge — the Emerge NOT_SELECTABLE
-- flip unmodeled; no unit-state bridge; no SpellScript/
-- AuraScript bridges — the nine script classes in this file
-- (spell_ahune_synch_health, spell_summoning_rhyme_aura,
-- spell_summon_ice_spear_delayer, spell_ice_spear_control_
-- aura, spell_ice_spear_target_picker, spell_slippery_floor_
-- periodic, spell_ahune_spanky_hands, spell_ahune_minion_
-- despawner, spell_ice_bombardment_dest_picker) are
-- documented only; npc_earthen_ring_flamecaller (25754) is
-- documented only — its whole AI is cross-creature driven
-- (SpellHit SHAMANS_LOOK_FOR_OPENING, DoAction EMOTE_
-- RESURFACE, Reset via the bunny's ResetFlameCallers) or
-- movement driven (MovementInform MovePoint arms) and would
-- be an empty skeleton; go_ahune_ice_stone (187882) is a
-- GameObjectScript — no GO bridge (lurker precedent);
-- SAY_PLAYER_TEXT_1/2/3 (0/1/2), EMOTE_EARTHEN_ASSAULT 0,
-- EMOTE_RETREAT 0, EMOTE_RESURFACE 1 and the Events/Actions/
-- Phases/Points/Misc enums are referenced only by the
-- unregistered arms above.

local SPELL_STAND = 37752
local SPELL_AHUNE_SPANKY_HANDS = 46146
local SPELL_AHUNES_SHIELD = 45954

local ENTRY_AHUNE = 25740

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
    states[guid] = nil
end

-- JustEngagedWith initial values: EVENT_SYNCH_HEALTH 3000.
local function freshState()
    return {
        synchHealth = 3000,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- EVENT_SYNCH_HEALTH: 3s init, re-arm 3s — timer
    -- bookkeeping only: the fire arm is instance->GetCreature
    -- (DATA_FROZEN_CORE) cross-creature gated (DoCast
    -- (frozenCore, 46430, true), else DoCastSelf(45254)), no
    -- cross-creature bridge (kalithresh precedent).
    if st.synchHealth <= 1000 then
        st.synchHealth = 3000
    else
        st.synchHealth = st.synchHealth - 1000
    end
end

RegisterCreatureEvent(ENTRY_AHUNE, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    -- EVENT_INITIAL_EMERGE (C++-scheduled at 4ms — fires
    -- directly on engage): DoCastSelf(SPELL_STAND),
    -- DoCastSelf(SPELL_AHUNE_SPANKY_HANDS), DoCastSelf
    -- (SPELL_AHUNES_SHIELD), C++-exact arm order.
    creature:CastSpell(nil, SPELL_STAND)
    creature:CastSpell(nil, SPELL_AHUNE_SPANKY_HANDS)
    creature:CastSpell(nil, SPELL_AHUNES_SHIELD)
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_AHUNE, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_AHUNE, 4, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_AHUNE, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
