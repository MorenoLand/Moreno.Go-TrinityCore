-- Eredar Twins (Sunwell Plateau) — Lua port of
-- src/server/scripts/EasternKingdoms/SunwellPlateau/boss_eredar_twins.cpp
-- (boss_sacrolash, boss_alythess, npc_shadow_image); sunwell_
-- plateau.h:34 (DATA_EREDAR_TWINS = 3, fourth boss), :42 (DATA_
-- ALYTHESS), :43 (DATA_SACROLASH), :76 (NPC_GRAND_WARLOCK_ALYTHESS
-- = 25166), :77 (NPC_SHADOW_IMAGE = 25214), :78 (NPC_LADY_
-- SACROLASH = 25165). Entries: 25165 Lady Sacrolash (C++ ScriptName
-- "boss_sacrolash"), 25166 Grand Warlock Alythess (C++ ScriptName
-- "boss_alythess"), 25214 Shadow Image (C++ ScriptName "npc_
-- shadow_image") — all entry+ScriptName verifiable from the C++
-- sources per RegisterSunwellPlateauCreatureAI in AddSC_boss_
-- eredar_twins; the creature_template ScriptName bindings are DB-
-- side — no TDB in this workspace, so only the C++-side naming is
-- verified. Talk lines used: Sacrolash YELL_SAC_KILL=8 (kill, 25%),
-- EMOTE_SHADOW_NOVA=5 + YELL_SHADOW_NOVA=9 (shadow nova);
-- Alythess EMOTE_CONFLAGRATION=4 + YELL_CANFLAGRATION=8
-- (conflagration), YELL_ALY_KILL=5 (kill, 25%), YELL_BERSERK=9
-- (berserk). YELL_INTRO_SAC_*=0/1/2/3 and YELL_INTRO_ALY_*=0/1/2/3
-- belong to the unmodeled intro chain; YELL_SAC_DEAD=4, YELL_SISTER_
-- ALYTHESS_DEAD=7, YELL_ALY_DEAD=6 and YELL_SISTER_SACROLASH_DEAD=7
-- belong to the sister-death arms (cross-creature blocked — below).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. Creature event 15 (OnSpellHitTarget) has a
-- constant in the scripting package but the world engine never
-- fires it — no bridge, so all SpellHitTarget touched-spell arms
-- are unmodeled (below).
-- Fight shape (C++-exact for the modeled arms): Lady Sacrolash
-- (25165). OnEnterCombat(1): per-GUID reset, arm shadow blades
-- 45248 10s then 10s, non-triggered self-cast (C++ DoCast(me, ...),
-- default triggered=false — vaelastrasz convention) / shadow nova
-- 45329 30s then {30s,35s}, non-triggered on a random alive player
-- in the instance (C++ SelectTarget(Random, 0), player-only — teron
-- convention; nil pick casts nothing but keeps the schedule),
-- Talk(EMOTE_SHADOW_NOVA) + Talk(YELL_SHADOW_NOVA) (the
-- !SisterDeath gate is always true on this path; the emote's C++
-- target arm has no target bridge — felmyst convention) /
-- confounding blow 45256 25s then {20s,25s}, non-triggered on a
-- random alive player in the instance, nil pick keeps the schedule
-- / enrage 46587 6min one-shot: Talk(YELL_ENRAGE), non-triggered
-- self-cast (C++ DoCast default triggered=false). The shadow-image
-- summon arm (3 x DoSpawnCreature NPC_SHADOW_IMAGE) has no summon
-- bridge — skipped (felmyst convention); the sister-death
-- conflagration arm 45342 has no cross-creature death bridge —
-- skipped (below). OnTargetDied(3): 25% Talk(YELL_SAC_KILL), no
-- TYPEID gate (C++-exact — the C++ KilledUnit Talk has no victim
-- gate). OnDied(4): the SisterDeath arm is cross-creature blocked —
-- documented; the loot-flag arm has no flag bridge. OnReset(23):
-- per-GUID reset (Enraged=false, SisterDeath=false) + cancel
-- timers; the instance sister respawn/threat relay and SetBossState
-- (NOT_STARTED) arms are blocked on the instance-script model.
-- OnLeaveCombat(2): cancel timers, drop per-GUID state. The
-- UpdateAI UNIT_STATE_CASTING queue gates and the InterruptSpell
-- arms have no UNIT_STATE/interrupt bridges — timers fire
-- unconditionally (jeklik convention). The SpellHitTarget
-- HandleTouchedSpells arm (45248/45329/45256/45270 -> dark touched
-- 45347; 45342 -> flame touched 45348) has no SpellHitTarget bridge
-- — unmodeled. The melee-range HandleTouchedSpells(victim, 45347)
-- arm has no melee-hook bridge — unmodeled (melee is engine-
-- driven). Grand Warlock Alythess (25166). OnEnterCombat(1):
-- per-GUID reset, arm conflagration 45342 45s then {30s,35s}, non-
-- triggered on a random alive player in the instance (nil pick
-- casts nothing but keeps the schedule), Talk(EMOTE_CONFLAGRATION)
-- + Talk(YELL_CANFLAGRATION), then the blaze timer is re-armed
-- once at 4s (C++-exact — the conflag arm sets BlazeTimer = 4000)
-- / flame sear 46771 15s then 15s, non-triggered self-cast /
-- pyrogenics 45230 15s then 15s, triggered self-cast (C++ DoCast
-- (me, SPELL_PYROGENICS, true), C++-exact) / blaze 45235 100ms then
-- 3.8s, non-triggered DoCastVictim (nil ticks cast nothing but keep
-- the schedule, jeklik convention) / enrage 46587 6min one-shot:
-- Talk(YELL_BERSERK), non-triggered self-cast. The sister-death
-- shadow-nova arm 45329 and the empower/sister-dead arms have no
-- cross-creature bridge — skipped. OnTargetDied(3): 25% Talk(YELL_
-- ALY_KILL), no TYPEID gate. OnDied(4): sister-death-gated Talk
-- (YELL_ALY_DEAD) blocked on the cross-creature bridge; the loot-
-- flag arm has no flag bridge. OnReset(23): per-GUID reset + cancel
-- timers; the instance sister respawn/threat relay and SetBossState
-- arms blocked on the instance-script model. OnLeaveCombat(2):
-- cancel timers, drop per-GUID state. The SpellHitTarget arm (45235
-- -> blaze-summon 45236 self-cast on the hit target; 45342/46771 ->
-- flame touched 45348; 45329 -> dark touched 45347) has no
-- SpellHitTarget bridge — unmodeled. The 8-step intro chain
-- (MoveInLineOfSight sighting + cross-creature Talk relays) has no
-- MoveInLineOfSight bridge — the fight starts on pull (ragnaros-
-- intro convention). The no-victim threat handoff from the sister
-- and the sister AttackStart relays have no cross-creature/threat
-- bridges — unmodeled. Shadow Image (25214). OnReset(23):
-- triggered self-cast image visual 45263 (C++-exact); the NOT_
-- SELECTABLE flag arm has no flag bridge. OnEnterCombat(1): arm
-- shadow fury 45270 {5s,20s} then 10s, non-triggered self-cast /
-- dark strike 45271 3s then 3s, non-triggered DoCastVictim (nil
-- ticks cast nothing but keep the schedule); a 10s visual-
-- maintenance check re-applies 45263 triggered if missing (the C++
-- per-tick HasAura arm, approximated). The 15s KillSelf arm has no
-- kill bridge — the image persists past 15s in this model. The
-- sacrolash-side DoSpawnCreature summon has no summon bridge, so
-- images arrive only via DB-side spawns — registered anyway for
-- its own verifiable AI arms (felmyst-trail convention).
-- Deliberate deviations (all await engine bridges): no instance-
-- script model — boss admission via the luaBossAI shim (BossAI::
-- JustEngagedWith/JustDied/Reset and the DATA_EREDAR_TWINS book-
-- keeping, DoZoneInCombat, sister AttackStart/respawn/threat relays
-- skipped); no cross-creature bridge — the sister-death detection
-- (instance->GetCreature(DATA_*), Talk(YELL_SISTER_*_DEAD), empower
-- 45366 self-cast, the sister-phase spell arms, the death Talk/
-- SetBossState(DONE) gates) and the alythess intro cross-Talk are
-- unmodeled; no SpellHitTarget bridge — all touched-spell chaining
-- and the alythess blaze-summon (GO 187366) arms unmodeled; no
-- summon bridge — the shadow-image spawns unmodeled; no flag/move/
-- threat/MoveInLineOfSight/kill bridges — the lootable-flag,
-- no-movement, AddThreat, intro-sighting and image KillSelf arms
-- unmodeled.

local SPELL_SHADOW_BLADES = 45248
local SPELL_SHADOW_NOVA = 45329
local SPELL_CONFOUNDING_BLOW = 45256
local SPELL_CONFLAGRATION = 45342
local SPELL_ENRAGE = 46587
local SPELL_PYROGENICS = 45230
local SPELL_FLAME_SEAR = 46771
local SPELL_BLAZE = 45235
local SPELL_IMAGE_VISUAL = 45263
local SPELL_SHADOW_FURY = 45270
local SPELL_DARK_STRIKE = 45271

local YELL_SAC_KILL = 8
local EMOTE_SHADOW_NOVA = 5
local YELL_SHADOW_NOVA = 9
local YELL_ENRAGE = 6
local EMOTE_CONFLAGRATION = 4
local YELL_CANFLAGRATION = 8
local YELL_ALY_KILL = 5
local YELL_BERSERK = 9

local timers = {}
local sacrolashState = {}
local alythessState = {}

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

-- Alive players sharing the creature's map+instance (teron
-- convention).
local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

local function pickRandomPlayer(creature)
    local players = playersInInstance(creature)
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- ================= Lady Sacrolash (25165) =================

-- C++ EVENT shadow blades: DoCastSelf(45248); repeat 10s.
local function sacrolashShadowBlades(creature, guid)
    creature:CastSpell(creature, SPELL_SHADOW_BLADES)
    schedule(guid, "shadowblades", 10000, function()
        sacrolashShadowBlades(creature, guid)
    end)
end

-- C++ EVENT shadow nova: DoCast(random target, 45329) +
-- Talk(EMOTE_SHADOW_NOVA) + Talk(YELL_SHADOW_NOVA); repeat
-- {30s,35s}. The !SisterDeath Talk gate is always true on this
-- path (sister-death arm is cross-creature blocked).
local function sacrolashShadowNova(creature, guid)
    local target = pickRandomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_NOVA)
        creature:Talk(EMOTE_SHADOW_NOVA)
        creature:Talk(YELL_SHADOW_NOVA)
    end
    schedule(guid, "shadownova", math.random(30000, 35000), function()
        sacrolashShadowNova(creature, guid)
    end)
end

-- C++ EVENT confounding blow: DoCast(random target, 45256);
-- repeat {20s,25s}.
local function sacrolashConfoundingBlow(creature, guid)
    local target = pickRandomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_CONFOUNDING_BLOW)
    end
    schedule(guid, "confoundingblow", math.random(20000, 25000), function()
        sacrolashConfoundingBlow(creature, guid)
    end)
end

-- C++ EVENT enrage: Talk(YELL_ENRAGE) + DoCastSelf(46587);
-- 6min one-shot, Enraged once-guard (C++-exact).
local function sacrolashEnrage(creature, guid)
    local st = sacrolashState[guid]
    if st and not st.enraged then
        st.enraged = true
        creature:Talk(YELL_ENRAGE)
        creature:CastSpell(creature, SPELL_ENRAGE)
    end
end

-- C++ JustEngagedWith + Initialize: per-GUID reset, arm the
-- five bridgeable timers.
local function sacrolashEnterCombat(event, creature)
    local guid = creature:GetGUID()
    sacrolashState[guid] = { enraged = false, sisterDeath = false }
    schedule(guid, "shadowblades", 10000, function()
        sacrolashShadowBlades(creature, guid)
    end)
    schedule(guid, "shadownova", 30000, function()
        sacrolashShadowNova(creature, guid)
    end)
    schedule(guid, "confoundingblow", 25000, function()
        sacrolashConfoundingBlow(creature, guid)
    end)
    schedule(guid, "enrage", 360000, function()
        sacrolashEnrage(creature, guid)
    end)
end

-- C++ KilledUnit: 25% Talk(YELL_SAC_KILL), no victim gate.
local function sacrolashTargetDied(event, creature)
    if math.random(1, 4) == 1 then
        creature:Talk(YELL_SAC_KILL)
    end
end

-- C++ JustDied: the SisterDeath arm is cross-creature blocked;
-- the lootable-flag arm has no flag bridge.
local function sacrolashDied(event, creature)
end

local function sacrolashLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    sacrolashState[guid] = nil
end

local function sacrolashReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    sacrolashState[guid] = nil
end

-- ================= Grand Warlock Alythess (25166) =================

-- C++ EVENT blaze: DoCastVictim(45235); repeat 3.8s. The
-- conflagration arm re-arms this once at 4s (C++-exact).
local function alythessBlaze(creature, guid)
    creature:CastSpell(nil, SPELL_BLAZE)
    schedule(guid, "blaze", 3800, function()
        alythessBlaze(creature, guid)
    end)
end

-- C++ EVENT conflagration: DoCast(random target, 45342) +
-- Talk(EMOTE_CONFLAGRATION) + Talk(YELL_CANFLAGRATION), BlazeTimer
-- = 4000; repeat {30s,35s}.
local function alythessConflagration(creature, guid)
    local target = pickRandomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_CONFLAGRATION)
        creature:Talk(EMOTE_CONFLAGRATION)
        creature:Talk(YELL_CANFLAGRATION)
    end
    schedule(guid, "blaze", 4000, function()
        alythessBlaze(creature, guid)
    end)
    schedule(guid, "conflagration", math.random(30000, 35000), function()
        alythessConflagration(creature, guid)
    end)
end

-- C++ EVENT flame sear: DoCastSelf(46771); repeat 15s.
local function alythessFlameSear(creature, guid)
    creature:CastSpell(creature, SPELL_FLAME_SEAR)
    schedule(guid, "flamesear", 15000, function()
        alythessFlameSear(creature, guid)
    end)
end

-- C++ EVENT pyrogenics: triggered DoCastSelf(45230); repeat 15s.
local function alythessPyrogenics(creature, guid)
    creature:CastSpell(creature, SPELL_PYROGENICS, true)
    schedule(guid, "pyrogenics", 15000, function()
        alythessPyrogenics(creature, guid)
    end)
end

-- C++ EVENT enrage: Talk(YELL_BERSERK) + DoCastSelf(46587);
-- 6min one-shot, Enraged once-guard (C++-exact).
local function alythessEnrage(creature, guid)
    local st = alythessState[guid]
    if st and not st.enraged then
        st.enraged = true
        creature:Talk(YELL_BERSERK)
        creature:CastSpell(creature, SPELL_ENRAGE)
    end
end

-- C++ JustEngagedWith + Initialize: per-GUID reset, arm the
-- five bridgeable timers.
local function alythessEnterCombat(event, creature)
    local guid = creature:GetGUID()
    alythessState[guid] = { enraged = false, sisterDeath = false }
    schedule(guid, "conflagration", 45000, function()
        alythessConflagration(creature, guid)
    end)
    schedule(guid, "flamesear", 15000, function()
        alythessFlameSear(creature, guid)
    end)
    schedule(guid, "pyrogenics", 15000, function()
        alythessPyrogenics(creature, guid)
    end)
    schedule(guid, "blaze", 100, function()
        alythessBlaze(creature, guid)
    end)
    schedule(guid, "enrage", 360000, function()
        alythessEnrage(creature, guid)
    end)
end

-- C++ KilledUnit: 25% Talk(YELL_ALY_KILL), no victim gate.
local function alythessTargetDied(event, creature)
    if math.random(1, 4) == 1 then
        creature:Talk(YELL_ALY_KILL)
    end
end

-- C++ JustDied: the sister-death-gated Talk(YELL_ALY_DEAD) arm is
-- cross-creature blocked; the lootable-flag arm has no flag
-- bridge.
local function alythessDied(event, creature)
end

local function alythessLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    alythessState[guid] = nil
end

local function alythessReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    alythessState[guid] = nil
end

-- ================= Shadow Image (25214) =================

-- C++ EVENT shadow fury: DoCastSelf(45270); first {5s,20s},
-- then 10s.
local function shadowImageFury(creature, guid)
    creature:CastSpell(creature, SPELL_SHADOW_FURY)
    schedule(guid, "shadowfury", 10000, function()
        shadowImageFury(creature, guid)
    end)
end

-- C++ EVENT dark strike: DoCastVictim(45271); repeat 3s.
local function shadowImageDarkStrike(creature, guid)
    creature:CastSpell(nil, SPELL_DARK_STRIKE)
    schedule(guid, "darkstrike", 3000, function()
        shadowImageDarkStrike(creature, guid)
    end)
end

-- C++ per-tick image-visual maintenance arm (approximated at
-- 10s): triggered self-cast 45263 if the aura is missing.
local function shadowImageVisualCheck(creature, guid)
    if not creature:HasAura(SPELL_IMAGE_VISUAL) then
        creature:CastSpell(creature, SPELL_IMAGE_VISUAL, true)
    end
    schedule(guid, "visualcheck", 10000, function()
        shadowImageVisualCheck(creature, guid)
    end)
end

-- C++ JustEngagedWith (empty in C++) + Initialize: arm the
-- combat timers. The 15s KillSelf arm has no kill bridge.
local function shadowImageEnterCombat(event, creature)
    local guid = creature:GetGUID()
    schedule(guid, "shadowfury", math.random(5000, 20000), function()
        shadowImageFury(creature, guid)
    end)
    schedule(guid, "darkstrike", 3000, function()
        shadowImageDarkStrike(creature, guid)
    end)
    schedule(guid, "visualcheck", 10000, function()
        shadowImageVisualCheck(creature, guid)
    end)
end

local function shadowImageLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset: triggered DoCastSelf(45263); the NOT_SELECTABLE
-- flag arm has no flag bridge.
local function shadowImageReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_IMAGE_VISUAL, true)
end

RegisterCreatureEvent(25165, 1, sacrolashEnterCombat)
RegisterCreatureEvent(25165, 2, sacrolashLeaveCombat)
RegisterCreatureEvent(25165, 3, sacrolashTargetDied)
RegisterCreatureEvent(25165, 4, sacrolashDied)
RegisterCreatureEvent(25165, 23, sacrolashReset)

RegisterCreatureEvent(25166, 1, alythessEnterCombat)
RegisterCreatureEvent(25166, 2, alythessLeaveCombat)
RegisterCreatureEvent(25166, 3, alythessTargetDied)
RegisterCreatureEvent(25166, 4, alythessDied)
RegisterCreatureEvent(25166, 23, alythessReset)

RegisterCreatureEvent(25214, 1, shadowImageEnterCombat)
RegisterCreatureEvent(25214, 2, shadowImageLeaveCombat)
RegisterCreatureEvent(25214, 23, shadowImageReset)
