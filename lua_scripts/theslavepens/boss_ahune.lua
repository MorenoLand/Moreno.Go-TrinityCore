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
-- EnterEvadeMode is instance/cross-creature/
-- summon gated (ahuneBunny DoAction STOP_EVENT,
-- summons.DespawnAll, DespawnOrUnsummon) — unmodeled.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (the DATA_AHUNE NOT_STARTED/IN_PROGRESS/DONE bookkeeping in
-- BossAI::JustEngagedWith instance bookkeeping / JustDied skipped
-- (base pulse + me-DoZone arms ported in ev1);
-- instance_the_slave_pens.cpp is PORTED 2026-11-10
-- (instance_the_slave_pens.lua: LoadObjectData entry latches +
-- flamecaller GUID capture via creature:SetGuidData, event 5);
-- no cross-creature bridge — the DoAction(ACTION_AHUNE_RETREAT) entry to
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
-- documented only — the Emerge NOT_SELECTABLE flip stays
-- unmodeled (its trigger rides the unregistered bunny phase
-- machine); BossAI base CASTING gate PORTED on the synch-health tick (see pump gate note; live creature:IsCasting bridge, lua_creature_events.go:1950); no SpellScript
-- bridge, and only one AuraScript bridge (RegisterAuraEvent,
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
-- triggers stay unregistered); go_ahune_ice_stone (187882) is a
-- GameObjectScript — RegisterGameObjectEvent exists, but the stone's
-- select body stays blocked (bunny DoAction + instance lookup);
-- EMOTE_EARTHEN_ASSAULT 0, EMOTE_RETREAT 0, EMOTE_RESURFACE 1
-- and the Events/Actions/Phases/Points/Misc enums are referenced
-- only by the unregistered arms above (SAY_PLAYER_TEXT_1/2/3
-- (0/1/2) ride the ported 45926 below).

local SPELL_STAND = 37752
local SPELL_AHUNE_SPANKY_HANDS = 46146
local SPELL_AHUNES_SHIELD = 45954

local ENTRY_AHUNE = 25740
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
    states[guid] = nil
end

-- JustEngagedWith initial values: EVENT_SYNCH_HEALTH 3000.
local function freshState()
    return {
        synchHealth = 3000,
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
    resetBoss(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_AHUNE, 4, function(_, creature)
    -- C++ JustDied (boss_ahune.cpp:197-201): Unit::Kill(bunny) then
    -- Unit::Kill(frozenCore); bunny 25745 (NPC_AHUNE_LOC_BUNNY),
    -- core 25865. FindNearestCreature 100.0 is the in-file
    -- instance-lookup idiom (:158). Killing the core fires its
    -- (already ported) ev4 — loot-missile + minion-despawner casts —
    -- C++-exact cascade. Recursion-safe: Kill on an already-dead
    -- target is a Health == 0 no-op (boss_ai.go:9697-9700).
    local bunny = creature:FindNearestCreature(25745, 100.0)
    if bunny then bunny:Kill(creature) end
    local core = creature:FindNearestCreature(ENTRY_FROZEN_CORE, 100.0)
    if core then core:Kill(creature) end
    resetBoss(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_AHUNE, 23, function(_, creature)
    resetBoss(creature:GetGUIDLow())
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
