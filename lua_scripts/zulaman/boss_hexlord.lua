-- Hex Lord Malacrass (Zul'Aman) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulAman/boss_hexlord.cpp
-- (boss_hex_lord_malacrassAI + boss_thurgAI + boss_alyson_antilleAI +
-- boss_gazakrothAI + boss_lord_raadanAI + boss_darkheartAI +
-- boss_slitherAI + boss_fenstalkerAI + boss_koraggAI); zulaman.h:32
-- (BOSS_HEXLORD = 4), zulaman.h:49 (NPC_HEXLORD = 24239).
-- Creature entries: 24239 Hex Lord Malacrass (C++ ScriptName
-- "boss_hexlord_malacrass" per AddSC_boss_hex_lord_malacrass); the eight
-- adds in C++ AddEntryList order — 24240 Alyson Antille
-- ("boss_alyson_antille"), 24241 Thurg ("boss_thurg"), 24242 Slither
-- ("boss_slither"), 24243 Lord Raadan ("boss_lord_raadan"), 24244
-- Gazakroth ("boss_gazakroth"), 24245 Fenstalker ("boss_fenstalker"),
-- 24246 Darkheart ("boss_darkheart"), 24247 Koragg ("boss_koragg").
-- The boss spawns NPC_TEMP_TRIGGER 23920 as the Siphon Soul vehicle; no
-- CreatureScript exists for it in this file (same entry as Jan'alai's
-- fire bomb, a different TDB ScriptName slot) and no combat events fire
-- for it — it is not registered. spell_hexlord_unstable_affliction is an
-- AuraScript; AuraScript handlers are not modeled (standing gap) and it
-- is not registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape: drain power 44131 (triggered self) every {40s,55s} with
-- Talk(YELL_DRAIN_POWER=3); spirit bolts 43383 at 20s — if the drain
-- timer has under 12s left it is deferred 13s, otherwise the cast runs
-- with Talk(YELL_SPIRIT_BOLTS=4), re-arms at 40s, cancels the ability
-- timer, and re-arms siphon soul at 10s; siphon soul 43501 at 100s / 10s
-- after each spirit bolts (the C++ 23920 trigger vehicle has no
-- summon-model bearer, so the boss casts the spell triggered on the
-- target directly): picks a random alive player within 70 yd (no threat
-- model) and records its class for the ability cycle; player abilities
-- every {8s,10s} from the stolen class's row (C++-exact 0-based row index
-- = class-1; monk lands on the druid row like C++; druid is guarded —
-- see deviations). Kills: random Talk(YELL_KILL_ONE=1/YELL_KILL_TWO=2)
-- (C++ KilledUnit has no player gate); death: Talk(YELL_DEATH=5).
-- Adds: Thurg — bloodlust 43578 15s then 12s (friendly-buff scan has no
-- bearer, cast skipped, 12s cycle kept), cleave 15496 10s then 12s on
-- victim; Alyson Antille — flash heal 43575 2.5s cycle (friendly scan has
-- no bearer) with the C++ else-branch dispel magic 43577 on a random
-- alive player 50% of ticks; Gazakroth — firebolt 43584 2s then 700ms on
-- victim; Lord Raadan — thunderclap 43583 13s then 12s / flame breath
-- 43582 8s then 12s on victim; Darkheart — psychic wail 43590 8s then
-- 12s on victim; Slither — venom spit 43579 5s then 2.5s on a random
-- alive player within 100 yd; Fenstalker — volatile infection 43586 15s
-- then 12s on victim; Koragg — mighty blow 43592 10s then 12s on victim /
-- cold stare 43593 15s then 12s on a random alive player within 100 yd.
-- The adds never spawn in this build (no summon model) but are
-- registered so any that exist fight correctly.
-- Deviations from C++: no summon model — the four adds never spawn, so
-- Reset's SpawnAdds, JustEngagedWith's add AttackStart arm (with its
-- EnterEvadeMode fallback), the 5s CheckAddState re-engage arm, and
-- JustDied's add KillSelf arm have no bearer; the 5s ResetTimer room-
-- check EnterEvadeMode has no bearer; the add base AI's DoZoneInCombat
-- and its GetBossState(IN_PROGRESS) evade gate have no bridge (no
-- instance-script model, so the BOSS_HEXLORD encounter bookkeeping never
-- runs); the SiphonSoul !target/!trigger EnterEvadeMode arm has no
-- bridge, so a missing siphon target is a no-op; the shadow-priest
-- HasSpell(15473) redirect has no bridge, so priests always use the holy
-- row; the C++ druid PlayerClass special case (class 11 -> index 11 of
-- the 10-row table) is genuine out-of-bounds upstream — Lua guards it
-- and steals no abilities from druids; HEAL-target and BUFF-target
-- abilities (DoSelectLowestHpFriendly / DoFindFriendlyMissingBuff) have
-- no bridge, so those casts are skipped while the ability cycle keeps
-- running; ABILITY_TARGET_SPECIAL (ice lance) falls through the C++
-- default to a random enemy, modeled exactly; thurg's bloodlust
-- friendly-buff scan has no bearer; alyson/slither 20-yd AttackStart
-- chase and gazakroth's AttackStartCaster have no movement bridge;
-- fenstalker's "core bug" victim-self-cast has no caster-override
-- bridge, so the boss-side creature casts on the victim; no threat model
-- — random targeting is threat-blind; the commented-out dispelmagic_timer
-- arm and the commented-out MoveChase fallback in Alyson's flashheal arm
-- are dead C++ code, not modeled; the commented-out target-alive check
-- in the PlayerAbility arm is upstream-dead, so abilities fire
-- unconditionally.

local ENTRY_HEXLORD = 24239
local ENTRY_ALYSON = 24240
local ENTRY_THURG = 24241
local ENTRY_SLITHER = 24242
local ENTRY_RAADAN = 24243
local ENTRY_GAZAKROTH = 24244
local ENTRY_FENSTALKER = 24245
local ENTRY_DARKHEART = 24246
local ENTRY_KORAGG = 24247

local YELL_AGGRO = 0
local YELL_KILL_ONE = 1
local YELL_KILL_TWO = 2
local YELL_DRAIN_POWER = 3
local YELL_SPIRIT_BOLTS = 4
local YELL_DEATH = 5

local SPELL_SPIRIT_BOLTS = 43383
local SPELL_DRAIN_POWER = 44131
local SPELL_SIPHON_SOUL = 43501

local SPELL_CLEAVE_ADD = 15496
local SPELL_BLOODLUST = 43578
local SPELL_FLASH_HEAL = 43575
local SPELL_DISPEL_MAGIC = 43577
local SPELL_FIREBOLT = 43584
local SPELL_FLAME_BREATH = 43582
local SPELL_THUNDERCLAP = 43583
local SPELL_PSYCHIC_WAIL = 43590
local SPELL_VENOM_SPIT = 43579
local SPELL_VOLATILE_INFECTION = 43586
local SPELL_COLD_STARE = 43593
local SPELL_MIGHTY_BLOW = 43592

-- C++ AbilityTarget: SELF=0, VICTIM=1, ENEMY=2, HEAL=3, BUFF=4, SPECIAL=5
-- (SPECIAL falls through the C++ default to a random enemy, so ice lance
-- is entered as ENEMY below).
local TGT_SELF = 0
local TGT_VICTIM = 1
local TGT_ENEMY = 2
local TGT_HEAL = 3
local TGT_BUFF = 4

-- C++ PlayerAbility[][3], 0-based index = player class id - 1 (TrinityCore
-- Classes enum: warrior=1 ... priest=5, death knight=6, shaman=7,
-- mage=8, warlock=9, monk=10, druid=11). Note the upstream quirks: death
-- knights land on the shadow-priest row and monks on the druid row
-- (both in-bounds in C++); druids index out of bounds (see deviations).
local ABILITIES = {
    [0] = { {43443, TGT_SELF}, {43442, TGT_SELF}, {43441, TGT_VICTIM} },   -- warrior
    [1] = { {43429, TGT_SELF}, {43451, TGT_HEAL}, {43430, TGT_SELF} },     -- paladin
    [2] = { {43444, TGT_SELF}, {43447, TGT_SELF}, {43449, TGT_SELF} },     -- hunter
    [3] = { {43461, TGT_VICTIM}, {43457, TGT_SELF}, {43433, TGT_ENEMY} },  -- rogue
    [4] = { {44416, TGT_HEAL}, {41372, TGT_HEAL}, {43432, TGT_SELF} },     -- priest
    [5] = { {43550, TGT_ENEMY}, {41374, TGT_ENEMY}, {41375, TGT_ENEMY} },  -- shadow priest (also DKs)
    [6] = { {43436, TGT_SELF}, {43548, TGT_HEAL}, {43435, TGT_ENEMY} },    -- shaman
    [7] = { {41383, TGT_ENEMY}, {43428, TGT_ENEMY}, {43427, TGT_ENEMY} },  -- mage
    [8] = { {43439, TGT_ENEMY}, {43440, TGT_ENEMY}, {43522, TGT_ENEMY} },  -- warlock
    [9] = { {43421, TGT_HEAL}, {43420, TGT_SELF}, {43545, TGT_ENEMY} },    -- druid
}

local timers = {}
local state = {}

local function cancelTimers(store, guid)
    local per = store[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        store[guid] = nil
    end
end

local function cancelTimer(store, guid, key)
    local per = store[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

local function schedule(store, guid, key, delay, fn)
    local per = store[guid]
    if not per then
        per = {}
        store[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function bossState(guid)
    local st = state[guid]
    if not st then
        st = { drainDeadline = 0, playerClass = nil, playerGuid = nil }
        state[guid] = st
    end
    return st
end

local function randomAlivePlayer(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

local onSpiritBolts
local onDrainPower
local onSiphonSoul
local onAbility

-- C++ drain power: triggered self-cast, Talk, repeat {40s,55s}. The
-- os.time deadline lets the spirit-bolts arm compare drain remaining
-- time (malchezaar convention).
onDrainPower = function(creature, guid)
    creature:CastSpell(creature, SPELL_DRAIN_POWER, true)
    creature:Talk(YELL_DRAIN_POWER)
    local delay = math.random(40000, 55000)
    bossState(guid).drainDeadline = os.time() + delay / 1000
    schedule(timers, guid, "drain", delay, function()
        onDrainPower(creature, guid)
    end)
end

-- C++ spirit bolts: if the drain timer has under 12s left, defer 13s
-- (cast drain power first); otherwise cast, Talk, re-arm 40s, cancel the
-- ability timer (PlayerAbility_Timer = 99999), and re-arm siphon soul.
onSpiritBolts = function(creature, guid)
    local remaining = bossState(guid).drainDeadline - os.time()
    if remaining < 12 then
        schedule(timers, guid, "spirit", 13000, function()
            onSpiritBolts(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SPIRIT_BOLTS, false)
    creature:Talk(YELL_SPIRIT_BOLTS)
    cancelTimer(timers, guid, "ability")
    schedule(timers, guid, "siphon", 10000, function()
        onSiphonSoul(creature, guid)
    end)
    schedule(timers, guid, "spirit", 40000, function()
        onSpiritBolts(creature, guid)
    end)
end

-- C++ siphon soul: random alive player within 70 yd (SelectTarget Random,
-- no threat model), boss casts the spell triggered on the target
-- directly (the 23920 trigger vehicle has no summon-model bearer), record
-- class and GUID, arm the ability cycle {8s,10s}; the siphon timer then
-- stays disarmed (99999) until the next spirit bolts.
onSiphonSoul = function(creature, guid)
    local target = randomAlivePlayer(creature, 70)
    if target then
        creature:CastSpell(target, SPELL_SIPHON_SOUL, true)
        local pc = target:GetClass() - 1
        if pc == 10 then
            pc = 11 -- C++ druid special case; out of bounds upstream — guarded below.
        end
        -- The shadow-priest HasSpell(15473) redirect has no bridge;
        -- priests keep the holy row.
        local st = bossState(guid)
        st.playerClass = pc
        st.playerGuid = target:GetGUID()
        schedule(timers, guid, "ability", math.random(8000, 10000), function()
            onAbility(creature, guid)
        end)
    end
end

-- C++ UseAbility: random of the three stolen-class abilities; the
-- upstream target-alive check is commented out, so the cycle re-arms
-- {8s,10s} unconditionally. HEAL/BUFF targets have no friendly-scan
-- bridge, so those casts are skipped.
onAbility = function(creature, guid)
    local st = bossState(guid)
    local row = st.playerClass and ABILITIES[st.playerClass] or nil
    if row then
        local pick = row[math.random(1, 3)]
        local spell, tgt = pick[1], pick[2]
        if tgt == TGT_SELF then
            creature:CastSpell(creature, spell, false)
        elseif tgt == TGT_VICTIM then
            creature:CastSpell(nil, spell, false)
        elseif tgt == TGT_ENEMY then
            local target = randomAlivePlayer(creature, 100)
            if target then
                creature:CastSpell(target, spell, false)
            end
        end
    end
    schedule(timers, guid, "ability", math.random(8000, 10000), function()
        onAbility(creature, guid)
    end)
end

local function hexlordEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    state[guid] = { drainDeadline = os.time() + 60, playerClass = nil, playerGuid = nil }
    creature:Talk(YELL_AGGRO)
    -- The C++ JustEngagedWith add AttackStart arm has no bearer (no
    -- summon model).
    schedule(timers, guid, "spirit", 20000, function()
        onSpiritBolts(creature, guid)
    end)
    schedule(timers, guid, "drain", 60000, function()
        onDrainPower(creature, guid)
    end)
    schedule(timers, guid, "siphon", 100000, function()
        onSiphonSoul(creature, guid)
    end)
    -- C++ PlayerAbility_Timer = 99999: the ability timer starts only
    -- after the first siphon soul.
end

local function hexlordLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    state[guid] = nil
end

local function hexlordTargetDied(event, creature, victim)
    creature:Talk(math.random(YELL_KILL_ONE, YELL_KILL_TWO))
end

local function hexlordDied(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    state[guid] = nil
    creature:Talk(YELL_DEATH)
    -- The C++ JustDied add KillSelf arm has no bearer (no summon model).
end

local function hexlordReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    state[guid] = nil
    -- C++ Reset's SpawnAdds has no bearer (no summon model); timers
    -- re-arm on the next combat.
end

RegisterCreatureEvent(ENTRY_HEXLORD, 1, hexlordEnterCombat)
RegisterCreatureEvent(ENTRY_HEXLORD, 2, hexlordLeaveCombat)
RegisterCreatureEvent(ENTRY_HEXLORD, 3, hexlordTargetDied)
RegisterCreatureEvent(ENTRY_HEXLORD, 4, hexlordDied)
RegisterCreatureEvent(ENTRY_HEXLORD, 23, hexlordReset)

-- The eight hexlord adds. The base boss_hexlord_addAI contributes only
-- DoZoneInCombat and the GetBossState(IN_PROGRESS) evade gate, both with
-- no bridge (no instance-script model), so each add is just its C++ cast
-- timers; melee is engine-driven. Table-driven: each arm is first delay,
-- repeat range, and fire closure.

local ADD_ARMS = {
    [ENTRY_THURG] = {
        -- C++ bloodlust: DoFindFriendlyMissingBuff has no bearer — cast
        -- skipped, 12s cycle kept (C++ re-arms 12000 unconditionally).
        { key = "bloodlust", first = 15000, min = 12000, max = 12000,
          fire = function(creature) end },
        { key = "cleave", first = 10000, min = 12000, max = 12000,
          fire = function(creature)
              creature:CastSpell(nil, SPELL_CLEAVE_ADD, false)
          end },
    },
    [ENTRY_ALYSON] = {
        -- C++ flashheal arm: DoSelectLowestHpFriendly(99, 30000) has no
        -- bearer; the else branch runs every tick instead — 50% random
        -- alive player (SelectTarget Random has no range gate) gets
        -- dispel magic. The commented-out MoveChase fallback is dead C++
        -- code, not modeled.
        { key = "flashheal", first = 2500, min = 2500, max = 2500,
          fire = function(creature)
              if math.random(0, 1) == 1 then
                  local target = randomAlivePlayer(creature, math.huge)
                  if target then
                      creature:CastSpell(target, SPELL_DISPEL_MAGIC, false)
                  end
              end
          end },
        -- The commented-out dispelmagic_timer arm is dead C++ code.
    },
    [ENTRY_GAZAKROTH] = {
        { key = "firebolt", first = 2000, min = 700, max = 700,
          fire = function(creature)
              creature:CastSpell(nil, SPELL_FIREBOLT, false)
          end },
    },
    [ENTRY_RAADAN] = {
        { key = "thunderclap", first = 13000, min = 12000, max = 12000,
          fire = function(creature)
              creature:CastSpell(nil, SPELL_THUNDERCLAP, false)
          end },
        { key = "flamebreath", first = 8000, min = 12000, max = 12000,
          fire = function(creature)
              creature:CastSpell(nil, SPELL_FLAME_BREATH, false)
          end },
    },
    [ENTRY_DARKHEART] = {
        { key = "psychicwail", first = 8000, min = 12000, max = 12000,
          fire = function(creature)
              creature:CastSpell(nil, SPELL_PSYCHIC_WAIL, false)
          end },
    },
    [ENTRY_SLITHER] = {
        { key = "venomspit", first = 5000, min = 2500, max = 2500,
          fire = function(creature)
              local target = randomAlivePlayer(creature, 100)
              if target then
                  creature:CastSpell(target, SPELL_VENOM_SPIT, false)
              end
          end },
    },
    [ENTRY_FENSTALKER] = {
        -- C++ "core bug": the victim casts the spell on itself. The
        -- bridge has no caster override, so the add casts on the victim.
        { key = "volatileinf", first = 15000, min = 12000, max = 12000,
          fire = function(creature)
              creature:CastSpell(nil, SPELL_VOLATILE_INFECTION, false)
          end },
    },
    [ENTRY_KORAGG] = {
        { key = "mightyblow", first = 10000, min = 12000, max = 12000,
          fire = function(creature)
              creature:CastSpell(nil, SPELL_MIGHTY_BLOW, false)
          end },
        { key = "coldstare", first = 15000, min = 12000, max = 12000,
          fire = function(creature)
              local target = randomAlivePlayer(creature, 100)
              if target then
                  creature:CastSpell(target, SPELL_COLD_STARE, false)
              end
          end },
    },
}

local addArmFire

addArmFire = function(creature, guid, arm)
    arm.fire(creature)
    local delay = arm.min
    if arm.max > arm.min then
        delay = math.random(arm.min, arm.max)
    end
    schedule(timers, guid, arm.key, delay, function()
        addArmFire(creature, guid, arm)
    end)
end

local function addEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    local arms = ADD_ARMS[creature:GetEntry()]
    if arms then
        for _, arm in ipairs(arms) do
            schedule(timers, guid, arm.key, arm.first, function()
                addArmFire(creature, guid, arm)
            end)
        end
    end
end

local function addLeaveCombat(event, creature)
    cancelTimers(timers, creature:GetGUID())
end

local function addDied(event, creature)
    cancelTimers(timers, creature:GetGUID())
end

local function addReset(event, creature)
    cancelTimers(timers, creature:GetGUID())
end

local ADD_ENTRIES = {
    ENTRY_ALYSON, ENTRY_THURG, ENTRY_SLITHER, ENTRY_RAADAN,
    ENTRY_GAZAKROTH, ENTRY_FENSTALKER, ENTRY_DARKHEART, ENTRY_KORAGG,
}

for _, entry in ipairs(ADD_ENTRIES) do
    RegisterCreatureEvent(entry, 1, addEnterCombat)
    RegisterCreatureEvent(entry, 2, addLeaveCombat)
    RegisterCreatureEvent(entry, 4, addDied)
    RegisterCreatureEvent(entry, 23, addReset)
end
