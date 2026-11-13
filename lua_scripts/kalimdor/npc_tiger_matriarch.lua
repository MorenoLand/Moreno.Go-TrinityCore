-- Durotar: Tiger Matriarch — Lua port of the KilledUnit and
-- DamageTaken arms of npc_tiger_matriarchAI in
-- src/server/scripts/Kalimdor/zone_durotar.cpp (class
-- npc_tiger_matriarch : public CreatureScript; GetAI returns a new
-- npc_tiger_matriarchAI; the ScriptName binding stays DB-side).
-- Entry: 40312 (boss_ai.go:18763 audit — "npc_tiger_matriarch
-- (40312) stays out"; the entry itself is verified there).
-- Eluna creature events: 3 OnTargetDied, 9 OnDamageTaken.
-- Ported arms (self-contained; neither uses _tigerGuid):
-- - KilledUnit (zone_durotar.cpp:183-195): victim must be TYPEID_UNIT
--   and a summon (the tiger vehicle 40305); the summon's summoner
--   (the quest player) loses NO_SUMMON_AURA 75213, DETECT_INVIS
--   75180, SPIRIT_OF_THE_TIGER_RIDER 75166 and SUMMON_ZENTABRA_TRIGGER
--   75212; the matriarch despawns. The DespawnOrUnsummon is outside
--   the vehSummoner null check in C++ — it runs even when the
--   summoner fails to resolve.
-- - DamageTaken (zone_durotar.cpp:198-222): attacker must be a summon
--   and the matriarch below 20% health (C++ HealthBelowPct(20) with
--   no damage argument — current-health check, and DamageTaken fires
--   pre-damage in both C++ and Lua ev9, so GetHealthPct() < 20 is
--   exact): damage = 0 (ev9 rewrite, `return false, 0` — reliquary /
--   malchezaar precedent), SetFlag(UNIT_FIELD_FLAGS,
--   UNIT_FLAG_NON_ATTACKABLE), the summon's summoner gains
--   SUMMON_ZENTABRA_TRIGGER 75212, triggered-cast SUMMON_ZENTABRA
--   75181 on itself, the attacker triggered-casts EJECT_PASSENGERS
--   50630 on itself, the summoner loses the four auras above, and
--   the matriarch despawns. The matriarch despawns on the first
--   qualifying hit, so the arm is one-shot in practice.
-- Stay-outs (documented in the boss_ai.go:18763 audit):
-- - IsSummonedBy (zone_durotar.cpp:174-181): rides GetVehicle — no
--   bridge — and sets _tigerGuid, AddThreat(tiger, 500000) and
--   FURIOUS_BITE 75164.
-- - UpdateAI (zone_durotar.cpp:224-262): POUNCE 61184 / NOSUMMON
--   50s re-apply of NO_SUMMON_AURA, both gated on _tigerGuid.
-- - JustEngagedWith (zone_durotar.cpp:164-169): _events.Reset() +
--   POUNCE 100ms / NOSUMMON 50s schedules (same _tigerGuid machine).
-- Note: the matriarch materializes via the credit NPC's triggered
-- SPELL_SUMMON_MATRIARCH 75187 cast (npc_tiger_matriarch_credit.lua),
-- whose summon-effect spawns nothing on the player path yet
-- (engine gap, documented there) — these arms are live the moment
-- that gap closes, and dormant until then (buru egg-dummy precedent).
local ENTRY = 40312

local TYPEID_UNIT = 3 -- SharedDefines.h TYPEID_UNIT (hyjal_trash precedent)

local UNIT_FIELD_FLAGS = 59 -- opera RemoveFlag precedent
local UNIT_FLAG_NON_ATTACKABLE = 0x2

local SPELL_NO_SUMMON_AURA = 75213 -- zone_durotar.cpp:43
local SPELL_DETECT_INVIS = 75180 -- zone_durotar.cpp:45
local SPELL_SUMMON_ZENTABRA_TRIGGER = 75212 -- zone_durotar.cpp:46
local SPELL_SUMMON_ZENTABRA = 75181 -- zone_durotar.cpp:50
local SPELL_SPIRIT_OF_THE_TIGER_RIDER = 75166 -- zone_durotar.cpp:52
local SPELL_EJECT_PASSENGERS = 50630 -- zone_durotar.cpp:53

-- C++ npc_tiger_matriarchAI::KilledUnit (zone_durotar.cpp:183-195):
-- victim must be TYPEID_UNIT and IsSummon; the summon's summoner
-- loses the four quest auras; the matriarch despawns unconditionally.
-- GetTypeId is the generic object fallback (scripting/object.go), so
-- it is safe on a player victim; IsSummon lives on the creature
-- surface only, so it is called after the TYPEID_UNIT gate.
local function onTargetDied(event, creature, victim)
    if not victim or victim:GetTypeId() ~= TYPEID_UNIT then
        return
    end
    if not victim:IsSummon() then
        return
    end
    local vehSummoner = victim:GetSummonerUnit()
    if vehSummoner then
        vehSummoner:RemoveAura(SPELL_NO_SUMMON_AURA)
        vehSummoner:RemoveAura(SPELL_DETECT_INVIS)
        vehSummoner:RemoveAura(SPELL_SPIRIT_OF_THE_TIGER_RIDER)
        vehSummoner:RemoveAura(SPELL_SUMMON_ZENTABRA_TRIGGER)
    end
    creature:Despawn()
end

-- C++ npc_tiger_matriarchAI::DamageTaken (zone_durotar.cpp:198-222):
-- attacker must be a summon and the matriarch below 20% health.
-- C++ order: damage = 0, SetFlag(NON_ATTACKABLE), summoner legs,
-- DespawnOrUnsummon. The ev9 rewrite rides the `return false, 0`
-- pair (fireCreatureDamageTaken, lua_creature_events.go:7670).
local function onDamageTaken(event, creature, attacker, damage)
    if not attacker or attacker:GetTypeId() ~= TYPEID_UNIT then
        return
    end
    if not attacker:IsSummon() then
        return
    end
    if creature:GetHealthPct() >= 20 then
        return
    end
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    local vehSummoner = attacker:GetSummonerUnit()
    if vehSummoner then
        vehSummoner:AddAura(SPELL_SUMMON_ZENTABRA_TRIGGER)
        vehSummoner:CastSpell(vehSummoner, SPELL_SUMMON_ZENTABRA, true)
        attacker:CastSpell(attacker, SPELL_EJECT_PASSENGERS, true)
        vehSummoner:RemoveAura(SPELL_NO_SUMMON_AURA)
        vehSummoner:RemoveAura(SPELL_DETECT_INVIS)
        vehSummoner:RemoveAura(SPELL_SPIRIT_OF_THE_TIGER_RIDER)
        vehSummoner:RemoveAura(SPELL_SUMMON_ZENTABRA_TRIGGER)
    end
    creature:Despawn()
    return false, 0
end

RegisterCreatureEvent(ENTRY, 3, onTargetDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
