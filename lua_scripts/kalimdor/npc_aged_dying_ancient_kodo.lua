-- Aged / Dying / Ancient Kodo (Desolace) --
-- Lua port of src/server/scripts/Kalimdor/zone_desolace.cpp
-- (npc_aged_dying_ancient_kodoAI — SpellHit buff arms only).
-- Zone-script unit per kalimdor_script_loader.cpp order
-- (ashenvale, azshara, azuremyst_isle, bloodmyst_isle,
-- darkshore closed; desolace now).
-- Entries (DyingKodo enum, verifiable from the C++ sources):
-- 4700 (NPC_AGED_KODO) / 4701 (NPC_DYING_KODO) /
-- 4702 (NPC_ANCIENT_KODO). The creature_template
-- ScriptName binding is DB-side (no TDB in this workspace).
-- Eluna creature events: 14 OnSpellHit (the 18153 buff arm).
-- C++ DoCast default is triggered=false (vaelastrasz
-- convention); the two modeled arms are triggered self-casts.
-- Fight shape (C++-exact for the modeled arms): OnSpellHit(14)
-- by SPELL_KODO_KOMBO_ITEM 18153 -> if the entry is one of the
-- three kodo entries and neither the caster has SPELL_KODO_
-- KOMBO_PLAYER_BUFF 18172 nor the kodo has SPELL_KODO_KOMBO_
-- DESPAWN_BUFF 18377: triggered self-cast 18172 on the caster
-- (player-side CastSpell bridge — modeled as caster:AddAura,
-- brutallus precedent) + triggered self-cast 18377 on the kodo.
-- Deliberate deviations (documented here): the 18153 arm's
-- mutation half — UpdateEntry(NPC_TAMED_KODO 11627), CombatStop,
-- SetFaction(FRIENDLY), SetSpeedRate(run 0.6f), EngagementOver,
-- MoveFollow, setActive(true), RemoveFlag(UNIT_NPC_FLAG_GOSSIP)
-- — has no entry-update / faction / movement / react / flag
-- bridges (phase_hunter / midnight precedents) — unmodeled;
-- the 18362 (SPELL_KODO_KOMBO_GOSSIP) SpellHit arm — SetFlag
-- gossip, SetHomePosition, MotionMaster Clear + MoveIdle,
-- setActive(false), DespawnOrUnsummon(60s) — has no flag /
-- movement / despawn bridges (terestian precedent) —
-- unmodeled; MoveInLineOfSight (the Smeed 11596 proximity
-- latch -> SPELL_KODO_KOMBO_GOSSIP 18362 triggered self-cast
-- + smeed Talk(SAY_SMEED_HOME 0)) has no proximity-detection
-- bridge (azshara depth-charge precedent) and the directed
-- cross-creature Talk has no bridge (omor precedent) —
-- unmodeled; OnGossipHello has no gossip bridge — unmodeled.
-- In the Go model the event-14 caster is always the player
-- session (no creature-caster spell path), so the C++
-- caster->ToUnit() gate always passes here.

local NPC_AGED_KODO = 4700
local NPC_DYING_KODO = 4701
local NPC_ANCIENT_KODO = 4702

local SPELL_KODO_KOMBO_ITEM = 18153
local SPELL_KODO_KOMBO_PLAYER_BUFF = 18172
local SPELL_KODO_KOMBO_DESPAWN_BUFF = 18377

local function kodoSpellHit(_, creature, caster, spellId)
    -- C++ SpellHit arm: SPELL_KODO_KOMBO_ITEM.
    if spellId ~= SPELL_KODO_KOMBO_ITEM then
        return
    end
    local entry = creature:GetEntry()
    if entry ~= NPC_AGED_KODO and entry ~= NPC_DYING_KODO
        and entry ~= NPC_ANCIENT_KODO then
        return
    end
    if caster:HasAura(SPELL_KODO_KOMBO_PLAYER_BUFF)
        or creature:HasAura(SPELL_KODO_KOMBO_DESPAWN_BUFF) then
        return
    end
    caster:AddAura(SPELL_KODO_KOMBO_PLAYER_BUFF)
    creature:CastSpell(creature, SPELL_KODO_KOMBO_DESPAWN_BUFF, true)
end

RegisterCreatureEvent(NPC_AGED_KODO, 14, kodoSpellHit)
RegisterCreatureEvent(NPC_DYING_KODO, 14, kodoSpellHit)
RegisterCreatureEvent(NPC_ANCIENT_KODO, 14, kodoSpellHit)
