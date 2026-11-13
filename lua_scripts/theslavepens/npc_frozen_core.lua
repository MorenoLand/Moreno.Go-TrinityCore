-- Ahune's Frozen Core (The Slave Pens, Midsummer Fire
-- Festival) — Lua port of src/server/scripts/Outland/
-- CoilfangReservoir/TheSlavePens/boss_ahune.cpp
-- (npc_frozen_core — the AI class AddSC_boss_ahune registers
-- under "npc_frozen_core"). Final Slave Pens port per
-- outland_script_loader.cpp order (mennu, rokmar, quagmirran,
-- ahune all closed).
-- Entry 25865 NPC_FROZEN_CORE (the_slave_pens.h) —
-- verifiable from the C++ sources: instance_the_slave_pens.
-- cpp maps { NPC_FROZEN_CORE, DATA_FROZEN_CORE };
-- DATA_FROZEN_CORE = 6 is an instance-side constant
-- (unbridgeable). The creature_template ScriptName bindings
-- are DB-side (no TDB in this workspace). The file has no
-- KilledUnit override — no event 3 registered.
-- Eluna creature events: 5 OnSpawn (engine-fired —
-- lua_creature_events.go), 2 OnLeaveCombat, 4 OnDied, 23
-- OnReset. C++ DoCast default is triggered=false
-- (vaelastrasz convention); DoCastSelf takes nil as the
-- target arm (felmyst convention).
-- Fight shape (C++-exact for all modeled arms): npc_frozen_
-- core (25865): OnSpawn(5): the ctor Initialize() — SetReactState
-- (REACT_PASSIVE) PORTED (stale "no react bridge" retracted — bridged
-- lua_creature_events.go:524, arlokk precedent) + SetRegenerateHealth
-- (false) PORTED (bridge added 2026-10-22, lua_creature_events.go) + the
-- two self casts DoCastSelf(SPELL_FROZEN_CORE_GETS_HIT 46810),
-- DoCastSelf(SPELL_ICE_SPEAR_AURA 46371). OnDied(4):
-- Unit::Kill(me, ahune) FIRST (boss_ahune.cpp:289-296) — PORTED tick
-- 386 via FindNearestCreature(25740, 100.0) + Kill (baltharus
-- precedent; the old "cross-creature blocked" claim is stale) —
-- then DoCastSelf(SPELL_SUMMON_LOOT_MISSILE 45941), DoCastSelf
-- (SPELL_MINION_DESPAWNER 46843) — C++-exact arm order
-- + cleanup. OnLeaveCombat(2)/OnReset(23): cleanup.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — the _instance GetCreature arms
-- skipped (the core reads its own motion only; the instance
-- guid store is latched by instance_the_slave_pens.lua). The
-- DoAction(ACTION_AHUNE_RETREAT/ACTION_AHUNE_RESURFACE) machine
-- is PORTED below (follow-up strip 2, unit D, 2026-11-13: ev38
-- receiver + phase-two EVENT_SYNCH_HEALTH pump) — the sender is
-- boss_ahune's Submerge()/Emerge() (boss_ahune.lua, same strip),
-- ev38 is the CreatureAI::DoAction dispatch twin
-- (lua_creature_events.go:837-859), and the inner arms ride live
-- bridges: SetFlag/RemoveFlag(59) (lua_creature_events.go:6401/
-- 6489), RemoveAura as the RemoveAurasDueToSpell twin
-- (lua_creature_events.go:1761), SetImmuneToPC as the
-- SetFlag/RemoveFlag(59, 0x100) twin (C++ Unit.cpp:8453-8475:
-- apply = SetFlag + ValidateAttackers + end player combat refs,
-- moot — the core is REACT_PASSIVE and never in combat; remove =
-- RemoveFlag exactly). The 46430 synch-health leg is modeled
-- directly via SetHealth (bossTick precedent in boss_ahune.lua).
-- The phase-two pump is a global CreateLuaEvent (owner 0, NOT
-- auto-cancelled on despawn — runtime.go:2425), so ev37 OnRemove
-- cancels it: the Ahune-evade despawn path never fires ev4. No
-- player bridge — the watery-grave style player arms have no
-- bridge (morogrim precedent).

local SPELL_FROZEN_CORE_GETS_HIT = 46810
local SPELL_ICE_SPEAR_AURA = 46371
local SPELL_SUMMON_LOOT_MISSILE = 45941
local SPELL_MINION_DESPAWNER = 46843
local SPELL_SYNCH_HEALTH = 46430 -- boss_ahune.cpp:39
local SPELL_SUICIDE = 45254 -- boss_ahune.cpp:88
local ACTION_AHUNE_RETREAT = -2586500 -- boss_ahune.cpp:132
local ACTION_AHUNE_RESURFACE = -2586501 -- boss_ahune.cpp:133
local UNIT_FIELD_FLAGS = 59 -- boss_curator.lua:37 precedent
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000 -- UnitDefines.h:149
local UNIT_FLAG_IMMUNE_TO_PC = 0x00000100 -- UnitDefines.h:132
local ENTRY_AHUNE = 25740

local ENTRY_FROZEN_CORE = 25865

local coreTimers = {}

local function coreCancelPump(guid)
    local id = coreTimers[guid]
    if id then
        RemoveEventById(id)
        coreTimers[guid] = nil
    end
end

-- C++ npc_frozen_core::UpdateAI EVENT_SYNCH_HEALTH (boss_ahune.cpp:324):
-- DoCast(ahune, SPELL_SYNCH_HEALTH, true), else DoCastSelf(SPELL_SUICIDE);
-- Repeat 3s. The 46430 script leg (hit health = caster health) is modeled
-- directly via SetHealth — the bossTick precedent in boss_ahune.lua (the
-- spell_ahune_synch_health SpellScript has no bridge). First fire at 3s,
-- matching the C++ ScheduleEvent(3s); repeats 0 = infinite (runtime.go
-- Tick: only repeats == 1 removes), cancelled by the RESURFACE leg and
-- the ev2/ev4/ev23/ev37 cancels below.
local function coreSynchTick(creature)
    local ahune = creature:FindNearestCreature(ENTRY_AHUNE, 100.0)
    if ahune then
        ahune:SetHealth(creature:GetHealth())
    else
        creature:CastSpell(creature, SPELL_SUICIDE)
    end
end

-- C++ ctor Initialize() (boss_ahune.cpp:282-287): SetReactState(
-- REACT_PASSIVE) first, then SetRegenerateHealth(false), then the two
-- self casts — C++ arm order.
RegisterCreatureEvent(ENTRY_FROZEN_CORE, 5, function(_, creature)
    creature:SetReactState(0) -- REACT_PASSIVE
    creature:SetRegenerateHealth(false)
    creature:CastSpell(creature, SPELL_FROZEN_CORE_GETS_HIT)
    creature:CastSpell(creature, SPELL_ICE_SPEAR_AURA)
end)

RegisterCreatureEvent(ENTRY_FROZEN_CORE, 2, function(_, creature)
    coreCancelPump(creature:GetGUIDLow())
end)

-- C++ JustDied (boss_ahune.cpp:289-296): Unit::Kill(me, ahune)
-- FIRST, then the two self casts. Recursion-safe: Kill on the
-- already-dead Ahune (the normal case — the core dies because Ahune
-- died) is a Health == 0 no-op (boss_ai.go:9697-9700). The pump cancel
-- is Lua bookkeeping, not a C++ arm: the phase-two pump is a global
-- timer that would outlive the corpse.
RegisterCreatureEvent(ENTRY_FROZEN_CORE, 4, function(_, creature)
    coreCancelPump(creature:GetGUIDLow())
    local ahune = creature:FindNearestCreature(ENTRY_AHUNE, 100.0)
    if ahune then ahune:Kill(creature) end
    creature:CastSpell(creature, SPELL_SUMMON_LOOT_MISSILE)
    creature:CastSpell(creature, SPELL_MINION_DESPAWNER)
end)

RegisterCreatureEvent(ENTRY_FROZEN_CORE, 23, function(_, creature)
    coreCancelPump(creature:GetGUIDLow())
end)

-- C++ npc_frozen_core::DoAction (boss_ahune.cpp:298-313): RETREAT →
-- RemoveFlag(NOT_SELECTABLE); SetImmuneToPC(false); RemoveAurasDueToSpell
-- (ICE_SPEAR_AURA); ScheduleEvent(EVENT_SYNCH_HEALTH, 3s, PHASE_TWO).
-- RESURFACE → _events.Reset(); DoCastSelf(ICE_SPEAR_AURA);
-- SetFlag(NOT_SELECTABLE); SetImmuneToPC(true). PORTED follow-up strip 2,
-- unit D. Sender: boss_ahune's Submerge()/Emerge() (boss_ahune.lua, same
-- strip); ev38 is the CreatureAI::DoAction dispatch twin
-- (lua_creature_events.go:837-859). SetImmuneToPC rides the
-- SetFlag/RemoveFlag(59, 0x100) twin (see header note). C++-exact arm order.
RegisterCreatureEvent(ENTRY_FROZEN_CORE, 38, function(_, creature, action)
    local guid = creature:GetGUIDLow()
    if action == ACTION_AHUNE_RETREAT then
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        creature:RemoveAura(SPELL_ICE_SPEAR_AURA)
        coreCancelPump(guid)
        coreTimers[guid] = CreateLuaEvent(function()
            coreSynchTick(creature)
        end, 3000, 0)
    elseif action == ACTION_AHUNE_RESURFACE then
        coreCancelPump(guid)
        creature:CastSpell(creature, SPELL_ICE_SPEAR_AURA)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
    end
end)

-- CreateLuaEvent timers are global (owner 0) and are NOT auto-cancelled
-- when the creature despawns (runtime.go:2425) — and the Ahune-evade
-- despawn path never fires ev4 — so ev37 OnRemove cancels the phase-two
-- pump (Despawn fires ev31 then ev37, lua_creature_events.go:3352-3362;
-- kiljaeden ev37 cleanup precedent). Idempotent with the ev2/ev4/ev23
-- cancels above.
RegisterCreatureEvent(ENTRY_FROZEN_CORE, 37, function(_, creature)
    coreCancelPump(creature:GetGUIDLow())
end)

-- C++ spell_ahune_minion_despawner::HandleScript
-- (boss_ahune.cpp:832-840, EFFECT_0 SPELL_EFFECT_APPLY_AURA, spell
-- 46843): the hit creature despawns. The cast is already ported
-- (OnDied above); the aura-apply leg is the carrier, not a
-- SpellScript hook — RegisterAuraEvent OnApply (event 2) fires once
-- per fresh application of 46843 on each hit creature
-- (serverApplyAuraToCreatureTarget), i.e. exactly the C++ hit set,
-- with no per-entry registration. PORTED tick 367. Caveat: the Go
-- hook is fresh-application-only while C++ fires on every hit; the
-- cast is one-shot (frozen-core death), so no refresh case exists in
-- practice.
local function minionDespawnerApply(event, aura, target)
    if target:GetObjectType() == "Creature" then
        target:Despawn()
    end
end

RegisterAuraEvent(SPELL_MINION_DESPAWNER, 2, minionDespawnerApply)
