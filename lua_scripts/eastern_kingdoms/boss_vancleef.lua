-- Boss VanCleef (Deadmines) --
-- Lua port of src/server/scripts/EasternKingdoms/Deadmines/boss_vancleef.cpp
-- (struct boss_vancleef : public BossAI, registered by AddSC_boss_vancleef
-- in eastern_kingdoms_script_loader.cpp via RegisterDeadminesCreatureAI ->
-- RegisterCreatureAIWithFactory; the script name is the stringified class
-- name "boss_vancleef" (koltira precedent, ScriptMgr.h:1234)).
-- Entry (verifiable from the C++ sources): NPC_VANCLEEF = 639 in the
-- DMCreaturesIds enum in Deadmines/deadmines.h (same-block header enum).
-- Whole-server-tree grep confirms boss_vancleef.cpp as the only source of
-- "boss_vancleef" (loader line in eastern_kingdoms_script_loader.cpp).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 5 OnSpawn, 9 OnDamageTaken, 23 OnReset. Melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Verifiable numbers (file's own enums): SPELL_DUAL_WIELD = 674 /
-- SPELL_THRASH = 12787 / SPELL_VANCLEEFS_ALLIES = 5200 (VanCleefData);
-- SAY_AGGRO = 0 / SAY_ONE = 1 / SAY_SUMMON = 2 / SAY_TWO = 3 /
-- SAY_KILL = 4 / SAY_THREE = 5 (Speech); BlackguardPositions (TDB coords):
-- (-78.2791, -824.784, 40.0007, 2.93215) /
-- (-77.8071, -815.097, 40.0188, 3.26377) x2 NPC_BLACKGUARD = 636
-- (deadmines.h), 1min TEMPSUMMON_CORPSE_TIMED_DESPAWN; health latches at
-- 66/33/25%; guards-called latch at 50%.
-- Ported arms:
-- - JustEngagedWith's Talk(SAY_AGGRO) -> creature:Talk(0) on
--   OnEnterCombat (1) (maiden convention); the summons.DoZoneInCombat()
--   leg strands on the unbridged summon surface (dark-rider precedent).
-- - KilledUnit's Talk(SAY_KILL) with the TYPEID_PLAYER gate ->
--   OnTargetDied (3) with a victim:IsPlayer() gate (illidan convention).
-- - DamageTaken's health latches (balinda OnDamageTaken(9) convention —
--   the handler fires with a fresh creature object built from live motion,
--   so (health - damage) is the genuine post-hit percent): 50% crossing
--   -> Talk(SAY_SUMMON = 2) + DoCastSelf(SPELL_VANCLEEFS_ALLIES = 5200)
--   ~ creature:CastSpell(creature, 5200) (one-shot via the per-guid
--   guardsCalled latch); 66% crossing -> Talk(SAY_ONE = 1); 33% crossing
--   -> Talk(SAY_TWO = 3); 25% crossing -> Talk(SAY_THREE = 5). The C++
--   checks the 50% leg in its own if, then 25/33/66 in an else-if chain —
--   replicated exactly; latches are one-shot per guid (init on 1/5/23).
-- Deviations from C++: the 50%-leg DoCastSelf(5200) spell's entire effect
-- is summoning allies — no summon bridge exists on the Lua surface
-- (dark-rider precedent), so only the cast arm is ported (creature casts
-- are packet-visual, maiden precedent); the allies never materialize.
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - Reset's DoCastSelf(SPELL_DUAL_WIELD = 674, true) +
--   DoCastSelf(SPELL_THRASH = 12787, true) — Reset self-casts are stranded
--   halves (rageclaw/phoenix_tk precedent: C++ Reset also runs at spawn
--   Initialize, while Lua OnReset (23) fires only on the reset/evade path;
--   no single faithful trigger).
-- - SummonBlackguards()'s DoSummon(NPC_BLACKGUARD = 636, the two
--   BlackguardPositions, 1min, TEMPSUMMON_CORPSE_TIMED_DESPAWN) x2 — no
--   summon bridge (dark-rider precedent).
-- - EnterEvadeMode's summons.DespawnAll() + _DespawnAtEvade() — no
--   summon/despawn bridge (dark-rider precedent).

local SPELL_VANCLEEFS_ALLIES = 5200

local SAY_AGGRO = 0
local SAY_ONE = 1
local SAY_SUMMON = 2
local SAY_TWO = 3
local SAY_KILL = 4
local SAY_THREE = 5

local ENTRY_VANCLEEF = 639

local latch = {}

local function initState(guid)
    latch[guid] = { guards = false, h25 = false, h33 = false, h66 = false }
end

local function clearState(guid)
    latch[guid] = nil
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local state = latch[guid]
    if not state then
        state = { guards = false, h25 = false, h33 = false, h66 = false }
        latch[guid] = state
    end
    local pct = (creature:GetHealth() - damage) * 100 / maxHealth
    if not state.guards and pct <= 50 then
        state.guards = true
        creature:Talk(SAY_SUMMON)
        creature:CastSpell(creature, SPELL_VANCLEEFS_ALLIES)
    end
    if not state.h25 and pct <= 25 then
        state.h25 = true
        creature:Talk(SAY_THREE)
    elseif not state.h33 and pct <= 33 then
        state.h33 = true
        creature:Talk(SAY_TWO)
    elseif not state.h66 and pct <= 66 then
        state.h66 = true
        creature:Talk(SAY_ONE)
    end
end

local function onTargetDied(_, creature, victim)
    if victim:IsPlayer() then
        creature:Talk(SAY_KILL)
    end
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    initState(guid)
    creature:Talk(SAY_AGGRO)
end

local function onCombatEnd(_, creature)
    clearState(creature:GetGUID())
end

local function onSpawn(_, creature)
    initState(creature:GetGUID())
end

local function onReset(_, creature)
    initState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_VANCLEEF, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_VANCLEEF, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_VANCLEEF, 3, onTargetDied)
RegisterCreatureEvent(ENTRY_VANCLEEF, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_VANCLEEF, 5, onSpawn)
RegisterCreatureEvent(ENTRY_VANCLEEF, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_VANCLEEF, 23, onReset)
