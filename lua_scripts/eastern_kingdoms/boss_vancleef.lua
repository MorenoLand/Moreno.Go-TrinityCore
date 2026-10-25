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
--   OnTargetDied (3) with a victim:GetObjectType() == "Player" gate (illidan convention).
-- - DamageTaken's health latches (the C++ arms read CURRENT health —
--   the engine's event 9 fires before damage is applied, matching C++
--   Unit::DealDamage, so the pre-damage read below is C++-exact): 50% crossing
--   -> Talk(SAY_SUMMON = 2) + DoCastSelf(SPELL_VANCLEEFS_ALLIES = 5200)
--   ~ creature:CastSpell(creature, 5200) (one-shot via the per-guid
--   guardsCalled latch); 66% crossing -> Talk(SAY_ONE = 1); 33% crossing
--   -> Talk(SAY_TWO = 3); 25% crossing -> Talk(SAY_THREE = 5). The C++
--   checks the 50% leg in its own if, then 25/33/66 in an else-if chain —
--   replicated exactly; latches are one-shot per guid (init on 1/5/23).
-- - Reset's DoCastSelf(SPELL_DUAL_WIELD = 674, true) +
--   DoCastSelf(SPELL_THRASH = 12787, true) -> onSpawn (5), alive-gated;
--   the triggered flag is exact via CastSpell's third arg (moroes
--   precedent). C++ Reset runs at spawn ONLY for this boss:
--   boss_vancleef's EnterEvadeMode override (boss_vancleef.cpp:78-84)
--   never calls the CreatureAI::EnterEvadeMode Reset() tail
--   (CreatureAI.cpp:237), so no Reset arms run on evade — the evade
--   path is despawn-only (summons.DespawnAll + _DespawnAtEvade 30s
--   respawn, ScriptedCreature.cpp:596), and the casts re-arm on the
--   respawn Reset.
-- - Reset's SummonBlackguards() (boss_vancleef.cpp:86-90: DoSummon(
--   NPC_BLACKGUARD = 636, the two BlackguardPositions, 1min,
--   TEMPSUMMON_CORPSE_TIMED_DESPAWN) x2) -> onSpawn (5, guarded) via
--   creature:SummonCreature(636, x, y, z, o, 5, 60000) (bridge:
--   lua_creature_events.go:956; type 5 = CORPSE_TIMED_DESPAWN per the
--   bridge's TempSummonType enum). The summons are tracked per boss
--   guid and despawned via creature:Despawn (bridge:
--   lua_creature_events.go:706) on evade (EnterEvadeMode's
--   summons.DespawnAll(), boss_vancleef.cpp:78-84) and on death
--   (BossAI::_JustDied's summons.DespawnAll(), ScriptedCreature.
--   cpp:516) — Go fires 23 ahead of 2/4, so the despawn rides onReset
--   DESPAWN-ONLY: an evade-path re-summon would contradict C++ (the
--   override skips the base Reset tail, so blackguards stay gone
--   until the 30s respawn Reset re-summons them). The 1min
--   idle-despawn of CORPSE_TIMED_DESPAWN is a documented bridge delta
--   (the spawn timer arms only for the timed family;
--   lua_creature_events.go:67).
-- Deviations from C++: the 50%-leg DoCastSelf(5200) spell's summon
-- effect rides the unbridged SPELL_EFFECT_SUMMON machine (the
-- creature:SummonCreature bridge exists, but the summoned positions
-- are spell data, not C++ — boss_vancleef.cpp only casts the spell),
-- so only Talk + the cast arm are ported (creature casts are
-- packet-visual, maiden precedent); the allies never materialize.
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - JustEngagedWith's summons.DoZoneInCombat() (boss_vancleef.cpp:69)
--   — no bridge (dark-rider precedent); the blackguards aggro via the
--   normal proximity scan.
-- - EnterEvadeMode's _DespawnAtEvade() (boss_vancleef.cpp:78-84) — no
--   bridge; the boss stays in world on evade (its summons ARE
--   despawned, see above).

local SPELL_DUAL_WIELD = 674
local SPELL_THRASH = 12787
local SPELL_VANCLEEFS_ALLIES = 5200

local SAY_AGGRO = 0
local SAY_ONE = 1
local SAY_SUMMON = 2
local SAY_TWO = 3
local SAY_KILL = 4
local SAY_THREE = 5

local ENTRY_VANCLEEF = 639
local ENTRY_BLACKGUARD = 636 -- NPC_BLACKGUARD (deadmines.h:78)

-- BlackguardPositions (boss_vancleef.cpp:40-43, TDB coords).
local BLACKGUARD_POSITIONS = {
    { -78.2791, -824.784, 40.0007, 2.93215 },
    { -77.8071, -815.097, 40.0188, 3.26377 },
}

local latch = {}
local blackguards = {}

local function initState(guid)
    latch[guid] = { guards = false, h25 = false, h33 = false, h66 = false }
end

local function clearState(guid)
    latch[guid] = nil
end

-- C++ Reset's SummonBlackguards() (boss_vancleef.cpp:86-90).
local function summonBlackguards(creature)
    local guid = creature:GetGUID()
    if blackguards[guid] then
        return
    end
    local list = {}
    for _, pos in ipairs(BLACKGUARD_POSITIONS) do
        -- TEMPSUMMON_CORPSE_TIMED_DESPAWN = 5, 1min (boss_vancleef.cpp:89).
        local bg = creature:SummonCreature(ENTRY_BLACKGUARD, pos[1], pos[2], pos[3], pos[4], 5, 60000)
        if bg then
            table.insert(list, bg)
        end
    end
    if #list > 0 then
        blackguards[guid] = list
    end
end

-- C++ EnterEvadeMode's summons.DespawnAll() (boss_vancleef.cpp:78-84)
-- + BossAI::_JustDied's summons.DespawnAll() (ScriptedCreature.cpp:516).
local function despawnBlackguards(creature)
    local guid = creature:GetGUID()
    local list = blackguards[guid]
    if not list then
        return
    end
    blackguards[guid] = nil
    for _, bg in ipairs(list) do
        bg:Despawn()
    end
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
    -- C++ DamageTaken HealthBelowPct(50/25/33/66) reads CURRENT
    -- health (jeklik convention): health < floor(maxHealth*pct/100).
    local health = creature:GetHealth()
    if not state.guards and health < math.floor(maxHealth * 50 / 100) then
        state.guards = true
        creature:Talk(SAY_SUMMON)
        creature:CastSpell(creature, SPELL_VANCLEEFS_ALLIES)
    end
    if not state.h25 and health < math.floor(maxHealth * 25 / 100) then
        state.h25 = true
        creature:Talk(SAY_THREE)
    elseif not state.h33 and health < math.floor(maxHealth * 33 / 100) then
        state.h33 = true
        creature:Talk(SAY_TWO)
    elseif not state.h66 and health < math.floor(maxHealth * 66 / 100) then
        state.h66 = true
        creature:Talk(SAY_ONE)
    end
end

local function onTargetDied(_, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    initState(guid)
    creature:Talk(SAY_AGGRO)
end

local function onLeaveCombat(_, creature)
    clearState(creature:GetGUID())
end

local function onDied(_, creature)
    -- C++ BossAI::_JustDied's summons.DespawnAll()
    -- (ScriptedCreature.cpp:516); the 23-ahead-of-4 despawn in onReset
    -- already ran, so this is a guard.
    despawnBlackguards(creature)
    clearState(creature:GetGUID())
end

local function onSpawn(_, creature)
    initState(creature:GetGUID())
    -- C++ Reset runs at spawn (Creature::Respawn -> ai->Reset(),
    -- Creature.cpp:2198); Go fires 23 ahead of 5 on the spawn funnel
    -- (fireCreatureSpawned, initial + respawn), so the Reset arms
    -- live HERE, not in onReset: vancleef's EnterEvadeMode override
    -- (boss_vancleef.cpp:78-84) never calls the CreatureAI::
    -- EnterEvadeMode Reset() tail, so Reset does NOT run on evade.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_DUAL_WIELD, true)
        creature:CastSpell(creature, SPELL_THRASH, true)
        summonBlackguards(creature)
    end
end

local function onReset(_, creature)
    initState(creature:GetGUID())
    -- Despawn-only: Go fires 23 ahead of 2 (evade) and ahead of 4
    -- (death), so the C++ EnterEvadeMode/_JustDied summon-despawn
    -- arms ride here — but NOT the Reset casts/summons (see
    -- onSpawn): vancleef's EnterEvadeMode override skips the base
    -- Reset tail, so an evade-path re-summon would contradict C++
    -- (blackguards stay gone until the 30s respawn Reset).
    despawnBlackguards(creature)
end

RegisterCreatureEvent(ENTRY_VANCLEEF, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_VANCLEEF, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_VANCLEEF, 3, onTargetDied)
RegisterCreatureEvent(ENTRY_VANCLEEF, 4, onDied)
RegisterCreatureEvent(ENTRY_VANCLEEF, 5, onSpawn)
RegisterCreatureEvent(ENTRY_VANCLEEF, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_VANCLEEF, 23, onReset)
