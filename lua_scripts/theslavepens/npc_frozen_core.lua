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
-- core (25865): OnSpawn(5): the ctor Initialize() self casts —
-- DoCastSelf(SPELL_FROZEN_CORE_GETS_HIT 46810), DoCastSelf
-- (SPELL_ICE_SPEAR_AURA 46371) (the SetReactState(REACT_
-- PASSIVE) arm has no react bridge — lurker precedent; the
-- SetRegenerateHealth(false) arm has no bridge). OnDied(4):
-- DoCastSelf(SPELL_SUMMON_LOOT_MISSILE 45941), DoCastSelf
-- (SPELL_MINION_DESPAWNER 46843) — C++-exact arm order
-- (the Unit::Kill(me, ahune) arm is cross-creature blocked —
-- instance->GetCreature(DATA_AHUNE), no cross-creature bridge)
-- + cleanup. OnLeaveCombat(2)/OnReset(23): cleanup.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — the _instance GetCreature arms
-- skipped (instance_the_slave_pens.cpp is a placeholder and
-- stays blocked on the instance-script model); no
-- cross-creature bridge — the DoAction(ACTION_AHUNE_RETREAT/
-- ACTION_AHUNE_RESURFACE) arms are unreachable: they are fired
-- only cross-creature by boss_ahune's Submerge()/Emerge(),
-- which are themselves unreachable in this model (the bunny
-- phase machine is unregistered), and their inner arms —
-- RemoveFlag(UNIT_FLAG_NOT_SELECTABLE), SetImmuneToPC,
-- RemoveAurasDueToSpell(SPELL_ICE_SPEAR_AURA) — have no flag/
-- immunity/per-spell-removal bridges anyway — so the whole
-- retreat/resurface state machine and the phase-two
-- EVENT_SYNCH_HEALTH event (DoCast(ahune, 46430, true), else
-- DoCastSelf(SPELL_SUICIDE 45254)) are documented only; no
-- player bridge — the watery-grave style player arms have no
-- bridge (morogrim precedent).

local SPELL_FROZEN_CORE_GETS_HIT = 46810
local SPELL_ICE_SPEAR_AURA = 46371
local SPELL_SUMMON_LOOT_MISSILE = 45941
local SPELL_MINION_DESPAWNER = 46843

local ENTRY_FROZEN_CORE = 25865

-- C++ ctor Initialize(): the two self casts. The react-state
-- and regenerate-health arms have no bridges.
RegisterCreatureEvent(ENTRY_FROZEN_CORE, 5, function(_, creature)
    creature:CastSpell(nil, SPELL_FROZEN_CORE_GETS_HIT)
    creature:CastSpell(nil, SPELL_ICE_SPEAR_AURA)
end)

RegisterCreatureEvent(ENTRY_FROZEN_CORE, 2, function(_, creature)
end)

-- C++ JustDied: the two DoCast(spellId) self casts
-- (C++-exact order); the Unit::Kill(me, ahune) arm is
-- cross-creature blocked.
RegisterCreatureEvent(ENTRY_FROZEN_CORE, 4, function(_, creature)
    creature:CastSpell(nil, SPELL_SUMMON_LOOT_MISSILE)
    creature:CastSpell(nil, SPELL_MINION_DESPAWNER)
end)

RegisterCreatureEvent(ENTRY_FROZEN_CORE, 23, function(_, creature)
end)
