-- Mother Shahraz (Black Temple) — Lua port of
-- src/server/scripts/Outland/BlackTemple/boss_mother_shahraz.cpp
-- (boss_mother_shahraz only; the fatal attraction / fatal
-- attraction link SpellScripts and the saber lash / generic
-- periodic / random periodic AuraScripts documented below, not
-- registered); black_temple.h:37 (DATA_MOTHER_SHAHRAZ = 6, seventh
-- boss), :77 (NPC_MOTHER_SHAHRAZ = 22947). Creature entry: 22947
-- Mother Shahraz (C++ ScriptName "boss_mother_shahraz" per
-- RegisterBlackTempleCreatureAI in AddSC_boss_mother_shahraz;
-- the creature_template ScriptName binding is DB-side — no TDB in
-- this workspace, so only the C++-side naming is verified). Talk
-- lines used: SAY_TAUNT=0 (taunt timer), SAY_AGGRO=1 (pull),
-- SAY_SPELL=2 (fatal attraction), SAY_SLAY=3 (kill, player-only),
-- SAY_ENRAGE=4 (10% enrage), SAY_DEATH=5 (death), EMOTE_ENRAGE=6
-- (10% enrage), EMOTE_BERSERK=7 (berserk); Talk(EMOTE_*, me) is
-- modeled as Talk(line) — the self target has no bridge.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken (pre-damage hook),
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat:
-- Talk(SAY_AGGRO), arm silencing shriek 40823 22s then {18s,30s},
-- non-triggered DoCastVictim (nil ticks cast nothing but keep the
-- schedule, jeklik convention) / prismatic shield {40880 shadow,
-- 40882 fire, 40883 nature, 40891 arcane, 40896 frost, 40897 holy}
-- 15s then 15s, non-triggered self-cast (C++ DoCast default is
-- triggered=false — vaelastrasz convention) / fatal attraction
-- Talk(SAY_SPELL) + non-triggered self-cast 40869 35s then 30s
-- (the SPELLVALUE_MAX_TARGETS=3 clamp has no bridge — the cast
-- goes out unclamped, teron convention; its targeting and the
-- 40871 damage-link arms live in the unmodeled SpellScripts) /
-- random beam {40863 sinister, 40865 vile, 40866 wicked, 40862
-- sinful} 6s then 30s, non-triggered self-cast (the periodic
-- trigger arms live in the unmodeled AuraScripts) / berserk 45078
-- 10min, non-triggered self-cast + Talk(EMOTE_BERSERK), one-shot /
-- taunt Talk(SAY_TAUNT) 35s then {30s,40s}, no cast (C++-exact).
-- OnDamageTaken(9, pre-damage hook): !_enraged and post-damage
-- health strictly below 10% (C++ HealthBelowPctDamaged — majordomo
-- convention) -> enrage: triggered self-cast random periodic
-- 40867 (C++ DoCastSelf(..., true)), Talk(EMOTE_ENRAGE) +
-- Talk(SAY_ENRAGE), once-guard via per-GUID state (C++ _enraged,
-- cleared on Reset). OnTargetDied(3): Talk(SAY_SLAY),
-- TYPEID_PLAYER gate (terestian convention). OnDied(4):
-- Talk(SAY_DEATH); the _JustDied instance arm is blocked on the
-- instance-script model. OnLeaveCombat(2)/OnReset(23): cancel
-- timers, drop per-GUID state (C++ Reset: _Reset/_enraged=false;
-- the EnterEvadeMode _DespawnAtEvade arm has no despawn bridge —
-- evade-side cleanup is engine-side).
-- The five spell/aura scripts registered by the C++ file have no
-- script bridges (standing gaps): spell_mother_shahraz_fatal_
-- attraction (40869 target filter to 3 enemies + TARGET_DEST_
-- CASTER_RANDOM teleport), spell_mother_shahraz_fatal_attraction_
-- link (40871 damage dummy), spell_mother_shahraz_saber_lash
-- (40816 random-target triggered periodic), spell_mother_shahraz_
-- generic_periodic (40863/40865/40866/40862 random-target
-- triggered periodics), spell_mother_shahraz_random_periodic
-- (40867 self-cast random beam periodic).
local ENTRY_MOTHER_SHAHRAZ = 22947

local SAY_TAUNT = 0
local SAY_AGGRO = 1
local SAY_SPELL = 2
local SAY_SLAY = 3
local SAY_ENRAGE = 4
local SAY_DEATH = 5
local EMOTE_ENRAGE = 6
local EMOTE_BERSERK = 7

local SPELL_BERSERK = 45078
local SPELL_FATAL_ATTRACTION_TELEPORT = 40869
local SPELL_SILENCING_SHRIEK = 40823
local SPELL_RANDOM_PERIODIC = 40867

local BeamTriggers = { 40863, 40865, 40866, 40862 }
local PrismaticAuras = { 40880, 40882, 40883, 40891, 40896, 40897 }

local timers = {}
local enraged = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ EVENT_SILENCING_SHRIEK: non-triggered DoCastVictim(40823);
-- re-arm {18s,30s}.
local function onSilencingShriek(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SILENCING_SHRIEK)
    end
    schedule(guid, "shriek", math.random(18000, 30000), function()
        onSilencingShriek(creature, guid)
    end)
end

-- C++ EVENT_PRISMATIC_SHIELD: non-triggered DoCastSelf(one of the
-- six prismatic auras, urand(0, 5)); re-arm 15s.
local function onPrismaticShield(creature, guid)
    creature:CastSpell(creature, PrismaticAuras[math.random(1, 6)])
    schedule(guid, "prismatic", 15000, function()
        onPrismaticShield(creature, guid)
    end)
end

-- C++ EVENT_FATAL_ATTRACTION: Talk(SAY_SPELL), non-triggered
-- DoCastSelf(40869, {SPELLVALUE_MAX_TARGETS, 3}); re-arm 30s. The
-- 3-target clamp and the teleport targeting live in the unmodeled
-- SpellScripts.
local function onFatalAttraction(creature, guid)
    creature:Talk(SAY_SPELL)
    creature:CastSpell(creature, SPELL_FATAL_ATTRACTION_TELEPORT)
    schedule(guid, "fatalattraction", 30000, function()
        onFatalAttraction(creature, guid)
    end)
end

-- C++ EVENT_RANDOM_BEAM: non-triggered DoCastSelf(one of the four
-- beam triggers, urand(0, 3)); re-arm 30s. The beam trigger arms
-- live in the unmodeled AuraScripts.
local function onRandomBeam(creature, guid)
    creature:CastSpell(creature, BeamTriggers[math.random(1, 4)])
    schedule(guid, "randombeam", 30000, function()
        onRandomBeam(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: Talk(EMOTE_BERSERK, me), non-triggered
-- DoCastSelf(45078); one-shot.
local function onBerserk(creature)
    creature:Talk(EMOTE_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK)
end

-- C++ EVENT_TAUNT: Talk(SAY_TAUNT); re-arm {30s,40s}. No cast.
local function onTaunt(creature, guid)
    creature:Talk(SAY_TAUNT)
    schedule(guid, "taunt", math.random(30000, 40000), function()
        onTaunt(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(SAY_AGGRO) + the six initial arms.
local function shahrazEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    enraged[guid] = false
    creature:Talk(SAY_AGGRO)
    schedule(guid, "shriek", 22000, function()
        onSilencingShriek(creature, guid)
    end)
    schedule(guid, "prismatic", 15000, function()
        onPrismaticShield(creature, guid)
    end)
    schedule(guid, "fatalattraction", 35000, function()
        onFatalAttraction(creature, guid)
    end)
    schedule(guid, "randombeam", 6000, function()
        onRandomBeam(creature, guid)
    end)
    schedule(guid, "berserk", 600000, function()
        onBerserk(creature)
    end)
    schedule(guid, "taunt", 35000, function()
        onTaunt(creature, guid)
    end)
end

-- C++ KilledUnit: player victim -> Talk(SAY_SLAY).
local function shahrazTargetDied(event, creature, victim)
    if victim:IsPlayer() then
        creature:Talk(SAY_SLAY)
    end
end

local function shahrazResetState(guid)
    cancelTimers(guid)
    enraged[guid] = nil
end

-- C++ JustDied: _JustDied (instance arm, no bridge) +
-- Talk(SAY_DEATH).
local function shahrazDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    shahrazResetState(creature:GetGUID())
end

-- C++ DamageTaken: !_enraged and HealthBelowPctDamaged(10, damage)
-- -> _enraged=true, triggered DoCastSelf(40867), Talk(EMOTE_ENRAGE)
-- + Talk(SAY_ENRAGE).
local function shahrazDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if enraged[guid] then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth < 10 then
        enraged[guid] = true
        creature:CastSpell(creature, SPELL_RANDOM_PERIODIC, true)
        creature:Talk(EMOTE_ENRAGE)
        creature:Talk(SAY_ENRAGE)
    end
end

local function shahrazLeaveCombat(event, creature)
    shahrazResetState(creature:GetGUID())
end

local function shahrazReset(event, creature)
    shahrazResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_MOTHER_SHAHRAZ, 1, shahrazEnterCombat)
RegisterCreatureEvent(ENTRY_MOTHER_SHAHRAZ, 2, shahrazLeaveCombat)
RegisterCreatureEvent(ENTRY_MOTHER_SHAHRAZ, 3, shahrazTargetDied)
RegisterCreatureEvent(ENTRY_MOTHER_SHAHRAZ, 4, shahrazDied)
RegisterCreatureEvent(ENTRY_MOTHER_SHAHRAZ, 9, shahrazDamageTaken)
RegisterCreatureEvent(ENTRY_MOTHER_SHAHRAZ, 23, shahrazReset)
