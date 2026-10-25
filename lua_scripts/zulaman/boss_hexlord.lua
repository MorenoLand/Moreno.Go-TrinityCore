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
-- target directly): picks a random threat-list player alive within 70 yd
-- (SelectTarget Random, 0, 70, true) and records its class for the ability cycle; player abilities
-- every {8s,10s} from the stolen class's row (C++-exact 0-based row index
-- = class-1; monk lands on the druid row like C++; druid is guarded —
-- see deviations). Kills: random Talk(YELL_KILL_ONE=1/YELL_KILL_TWO=2)
-- (C++ KilledUnit has no player gate); death: Talk(YELL_DEATH=5).
-- Adds: Thurg — bloodlust 43578 15s then 12s on the first in-combat
-- friendly in 50 yd missing the aura (firstMissingBuff), cleave 15496
-- 10s then 12s on victim; Alyson Antille — flash heal 43575 on the
-- most-missing friendly in 99 yd (>30000 missing HP) when within 50 yd,
-- and the dispel magic 43577 else-branch fires on the urand(0,1)==1
-- most-missing-friendly-in-50-yd leg or the urand==0 random-threat-target
-- leg; Gazakroth — firebolt 43584 2s
-- then 700ms on victim; Lord Raadan — thunderclap 43583 13s then 12s / flame breath
-- 43582 8s then 12s on victim; Darkheart — psychic wail 43590 8s then
-- 12s on victim; Slither — venom spit 43579 5s then 2.5s on a random
-- threat-list player alive within 100 yd; Fenstalker — volatile infection 43586 15s
-- then 12s on victim; Koragg — mighty blow 43592 / cold stare 43593
-- are upstream-dead (timers never decrement, boss_hexlord.cpp:984-993)
-- and are not modeled.
-- The four adds spawn on Reset via the live SummonCreature bridge
-- (lua_creature_events.go:877): the ctor's SelectAddEntry picks 4 of the
-- 8 once (C++ RandomResize) and Reset's SpawnAdds respawns the missing/
-- dead ones at (Pos_X[i], 921.2795, 33.8883, 1.5696)
-- (TEMPSUMMON_DEAD_DESPAWN, Lua type 6); a living add's
-- EnterEvadeMode+reposition+StopMoving leg has no
-- EnterEvadeMode/UpdatePosition bridge, so living adds stay in place.
-- The eight adds' class AIs are registered in this file so the summons
-- fight.
-- Deviations from C++: JustEngagedWith's add AttackStart arm, the 5s
-- CheckAddState re-engage arm, and JustDied's add KillSelf arm are
-- PORTED 2026-10-24 via EngageWithTarget / creature:Kill() (the
-- EnterEvadeMode fallback on a missing/dead add still has no bridge,
-- so a missing add is skipped and the remaining adds still engage);
-- the 5s ResetTimer room-
-- check EnterEvadeMode has no bearer; the add base AI's
-- GetBossState(IN_PROGRESS) evade gate has no bridge (no
-- instance-script model, so the BOSS_HEXLORD encounter bookkeeping never
-- runs) — its DoZoneInCombat arm is ported below (bridge live 2026-10-23); the SiphonSoul !target/!trigger EnterEvadeMode arm has no
-- bridge, so a missing siphon target is a no-op; the shadow-priest
-- HasSpell(15473) redirect is ported via the player:HasSpell bridge;
-- the C++ druid PlayerClass special case (class 11 -> index 11 of
-- the 10-row table) is genuine out-of-bounds upstream — Lua guards it
-- and steals no abilities from druids; HEAL-target abilities
-- (DoSelectLowestHpFriendly) scan the friendly set via lowestHpFriendly
-- while the ability cycle keeps running; the BUFF leg has no rows in
-- the upstream PlayerAbility table; ABILITY_TARGET_SPECIAL (ice lance)
-- falls through the C++ default to a random enemy, modeled exactly;
-- thurg's bloodlust friendly-buff scan is ported via firstMissingBuff;
-- alyson/slither 20-yd AttackStart
-- chase and gazakroth's AttackStartCaster have no movement bridge;
-- fenstalker's "core bug" victim-self-cast has no caster-override
-- bridge, so the boss-side creature casts on the victim; random target
-- pools are threat-list (siphon/ability/slither/alyson); the commented-out dispelmagic_timer
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

local ENTRY_TEMP_TRIGGER = 23920
local MODEL_INVISIBLE = 11686
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

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
local hexlordAddIds

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

-- C++ SelectTarget(Random, 0) with playerOnly=FALSE (UnitAI.h:170):
-- random over the threat list, pets/totems eligible, dead entries skipped
-- (C++ SelectTargetList skip). Bridged via
-- creature:GetAITargets() (threat-ordered; lua_creature_events.go:345),
-- the boss_midnight randomTargetAnywhere precedent.
local function randomThreatTarget(creature)
    local targets = creature:GetAITargets()
    local out = {}
    for _, unit in ipairs(targets) do
        if not unit:IsDead() then
            out[#out + 1] = unit
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
end

-- C++ SelectTarget(Random, 0, maxDist, true): threat-list players alive
-- within maxDist (akilzon idiom).
local function randomThreatPlayerInRange(creature, maxDist)
    local targets = creature:GetAITargets()
    local out = {}
    for _, unit in ipairs(targets) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) <= maxDist then
            out[#out + 1] = unit
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
end

-- The boss's friendly set: itself plus the eight adds (zulaman.h:49,
-- boss_hexlord.cpp AddEntryList order). Only four of the eight spawn per
-- fight (the ctor's RandomResize pick); the grid scan simply finds the
-- ones present. Entry-filtering replaces the C++ !IsHostileTo check
-- (sulfuron precedent — these entries are the known-friendly set).
local FRIENDLY_ENTRIES = {
    ENTRY_HEXLORD, ENTRY_ALYSON, ENTRY_THURG, ENTRY_SLITHER,
    ENTRY_RAADAN, ENTRY_GAZAKROTH, ENTRY_FENSTALKER,
    ENTRY_DARKHEART, ENTRY_KORAGG,
}

-- C++ ScriptedAI::DoSelectLowestHpFriendly(range, minHPDiff)
-- (GridNotifiers.h:813): alive, in combat, not hostile to the searcher,
-- within range, missing strictly more than minHPDiff HP; the strictly
-- most-missing unit wins. Scan order here is entry then GUID (the C++
-- cell-visit order is unspecified).
local function lowestHpFriendly(creature, range, minMissing)
    local best, bestMissing = nil, minMissing
    for _, entry in ipairs(FRIENDLY_ENTRIES) do
        for _, add in ipairs(creature:GetCreaturesInGrid(entry, range)) do
            if not add:IsDead() and add:IsInCombat() then
                local missing = add:GetMaxHealth() - add:GetHealth()
                if missing > bestMissing then
                    best, bestMissing = add, missing
                end
            end
        end
    end
    return best
end

-- C++ ScriptedAI::DoFindFriendlyMissingBuff(range, spell) first-pick
-- (ScriptedCreature.cpp:444; GridNotifiers.h:897): first alive, in-combat
-- friendly creature in range missing the aura. Same entry/GUID order
-- caveat as lowestHpFriendly (the stale "no friendly-scan bridge" claim
-- is retracted — GetCreaturesInGrid + HasAura are live).
local function firstMissingBuff(creature, range, spell)
    for _, entry in ipairs(FRIENDLY_ENTRIES) do
        for _, add in ipairs(creature:GetCreaturesInGrid(entry, range)) do
            if not add:IsDead() and add:IsInCombat()
                    and not add:HasAura(spell) then
                return add
            end
        end
    end
    return nil
end

local onSpiritBolts
local onDrainPower
local onSiphonSoul
local onAbility
local engagePickedAdds
local checkAddState

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

-- C++ siphon soul: threat-list player alive within 70 yd (SelectTarget
-- Random, 0, 70, true), DoSpawnCreature(NPC_TEMP_TRIGGER 23920, 0,0,0,0,
-- TEMPSUMMON_TIMED_DESPAWN 30s) + SetDisplayId(11686 invisible) +
-- SetFlag(NOT_SELECTABLE) + trigger casts the spell triggered on the
-- target (MoveChase stays out: no motion bridge, so the trigger stands
-- at the boss close point instead of chasing the boss), record class
-- and GUID, arm the ability cycle {8s,10s}; the siphon timer then
-- stays disarmed (99999) until the next spirit bolts. Nil-summon
-- fallback keeps the old boss-direct cast (C++ would EnterEvadeMode
-- on a failed spawn; no evade bridge).
onSiphonSoul = function(creature, guid)
    local target = randomThreatPlayerInRange(creature, 70)
    if target then
        local trigger = creature:SummonCreature(ENTRY_TEMP_TRIGGER, 0, 0, 0, 0, 2, 30000)
        if trigger then
            trigger:SetDisplayId(MODEL_INVISIBLE)
            trigger:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            trigger:CastSpell(target, SPELL_SIPHON_SOUL, true)
        else
            creature:CastSpell(target, SPELL_SIPHON_SOUL, true)
        end
        local pc = target:GetClass() - 1
        if pc == 10 then
            pc = 11 -- C++ druid special case; out of bounds upstream — guarded below.
        elseif pc == 4 and target:HasSpell(15473) then
            pc = 5 -- C++ shadow-priest redirect (boss_hexlord.cpp:446).
        end
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
-- {8s,10s} unconditionally. HEAL targets scan the friendly set via
-- lowestHpFriendly (the stale "no friendly-scan bridge" claim is
-- retracted); the C++ PlayerAbility table has no ABILITY_TARGET_BUFF
-- rows, so the BUFF leg never fires upstream.
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
            local target = randomThreatPlayerInRange(creature, 100)
            if target then
                creature:CastSpell(target, spell, false)
            end
        elseif tgt == TGT_HEAL then
            -- C++ DoSelectLowestHpFriendly(50.f, 0): the most-HP-missing
            -- friendly in 50 yd; no cast when nobody is hurt.
            local target = lowestHpFriendly(creature, 50.0, 0)
            if target then
                creature:CastSpell(target, spell, false)
            end
        end
    end
    schedule(timers, guid, "ability", math.random(8000, 10000), function()
        onAbility(creature, guid)
    end)
end

local function hexlordEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    state[guid] = { drainDeadline = os.time() + 60, playerClass = nil, playerGuid = nil }
    creature:Talk(YELL_AGGRO)
    -- C++ JustEngagedWith (boss_hexlord.cpp:290-300): each picked add
    -- engages the boss's victim (EnterEvadeMode fallback stays out —
    -- no bridge — so a missing add is skipped, see engagePickedAdds).
    engagePickedAdds(creature, target or creature:GetVictim())
    -- C++ CheckAddState_Timer = 5000 (boss_hexlord.cpp:383-390).
    schedule(timers, guid, "checkadds", 5000, function()
        checkAddState(creature, guid)
    end)
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
    -- C++ JustDied's add KillSelf arm (boss_hexlord.cpp:326-331):
    -- each picked add alive is killed (grid scan stands in for the
    -- stored AddGUID[] array — no GUID->creature bridge).
    local ids = hexlordAddIds[guid]
    if ids then
        for _, entry in ipairs(ids) do
            for _, add in ipairs(creature:GetCreaturesInGrid(entry, 150)) do
                if not add:IsDead() then
                    add:Kill()
                end
            end
        end
    end
end

-- C++ boss_hexlord.cpp:134-141: Pos_X[4], POS_Y 921.2795, POS_Z 33.8883,
-- ORIENT 1.5696; AddEntryList[8] = 24240..24247 (AddCount 8). The ctor
-- calls SelectAddEntry() once — RandomResize to 4 — and the 4 picks
-- persist on the AI across resets, so they live in a table outside
-- state (state is wiped on Reset/leave/death).
local HEXLORD_SPAWN_X = {112.8827, 107.8827, 122.8827, 127.8827}
local HEXLORD_SPAWN_Y, HEXLORD_SPAWN_Z, HEXLORD_SPAWN_O = 921.2795, 33.8883, 1.5696
local HEXLORD_ADD_POOL = {24240, 24241, 24242, 24243, 24244, 24245, 24246, 24247}
hexlordAddIds = {}

-- C++ JustEngagedWith (boss_hexlord.cpp:290-300): for each of the 4
-- picked adds, if alive, AI()->AttackStart(me->GetVictim()); else
-- EnterEvadeMode + break. The grid scan stands in for AddGUID[]
-- (no GUID->creature bridge); the 150 yd scan is wider than Reset's
-- 60 yd so boss-to-add distance cannot hide an add (the eight add
-- entries are fight-unique, so no false positives). The evade leg
-- has no bridge: a missing/dead add is skipped and the remaining
-- alive adds still engage instead of breaking.
engagePickedAdds = function(creature, victim)
    if victim == nil then
        return
    end
    local ids = hexlordAddIds[creature:GetGUID()]
    if not ids then
        return
    end
    for _, entry in ipairs(ids) do
        for _, add in ipairs(creature:GetCreaturesInGrid(entry, 150)) do
            if not add:IsDead() then
                add:EngageWithTarget(victim)
            end
        end
    end
end

-- C++ CheckAddState (boss_hexlord.cpp:383-390): every 5s, each picked
-- add alive with no victim re-engages the boss's victim.
checkAddState = function(creature, guid)
    local victim = creature:GetVictim()
    if victim ~= nil then
        local ids = hexlordAddIds[guid]
        if ids then
            for _, entry in ipairs(ids) do
                for _, add in ipairs(creature:GetCreaturesInGrid(entry, 150)) do
                    if not add:IsDead() and add:GetVictim() == nil then
                        add:EngageWithTarget(victim)
                    end
                end
            end
        end
    end
    schedule(timers, guid, "checkadds", 5000, function()
        checkAddState(creature, guid)
    end)
end

local function hexlordReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    state[guid] = nil
    -- C++ Reset (boss_hexlord.cpp:282-288): _Reset(); Initialize();
    -- SpawnAdds(). SpawnAdds (:344-364): for each of the 4 picked
    -- adds, respawn at (Pos_X[i], POS_Y, POS_Z, ORIENT)
    -- (TEMPSUMMON_DEAD_DESPAWN, Lua type 6) when the add is missing or
    -- dead (a dead add is set DEAD first — the corpse is swept here by
    -- entry); a living add gets EnterEvadeMode + reposition +
    -- StopMoving — the reposition/StopMoving legs are ported via
    -- grid-add Teleport + StopMoving (Teleport is the NearTeleportTo
    -- twin: UpdatePosition + spline clear; EnterEvadeMode itself has
    -- no bridge, so threat/combat state is untouched).
    -- Timers re-arm on the next combat.
    if hexlordAddIds[guid] == nil then
        local pool = {24240, 24241, 24242, 24243, 24244, 24245, 24246, 24247}
        local picked = {}
        for i = 1, 4 do
            local idx = math.random(#pool)
            picked[i] = pool[idx]
            table.remove(pool, idx)
        end
        hexlordAddIds[guid] = picked
    end
    local ids = hexlordAddIds[guid]
    for i = 1, 4 do
        local entry = ids[i]
        local alive = false
        for _, add in ipairs(creature:GetCreaturesInGrid(entry, 60)) do
            if not add:IsDead() then
                alive = true
                break
            end
        end
        if not alive then
            for _, add in ipairs(creature:GetCreaturesInGrid(entry, 60)) do
                add:Despawn()
            end
            creature:SummonCreature(entry, HEXLORD_SPAWN_X[i], HEXLORD_SPAWN_Y, HEXLORD_SPAWN_Z, HEXLORD_SPAWN_O, 6)
        else
            for _, add in ipairs(creature:GetCreaturesInGrid(entry, 60)) do
                if not add:IsDead() then
                    add:Teleport(HEXLORD_SPAWN_X[i], HEXLORD_SPAWN_Y, HEXLORD_SPAWN_Z, HEXLORD_SPAWN_O)
                    add:StopMoving()
                end
            end
        end
    end
end

RegisterCreatureEvent(ENTRY_HEXLORD, 1, hexlordEnterCombat)
RegisterCreatureEvent(ENTRY_HEXLORD, 2, hexlordLeaveCombat)
RegisterCreatureEvent(ENTRY_HEXLORD, 3, hexlordTargetDied)
RegisterCreatureEvent(ENTRY_HEXLORD, 4, hexlordDied)
RegisterCreatureEvent(ENTRY_HEXLORD, 23, hexlordReset)

-- The eight hexlord adds. The base boss_hexlord_addAI contributes only
-- the GetBossState(IN_PROGRESS) evade gate, which has no bridge (no
-- instance-script model) — its DoZoneInCombat is ported in addEnterCombat
-- below (bridge live 2026-10-23) — so each add is just its C++ cast
-- timers; melee is engine-driven. Table-driven: each arm is first delay,
-- repeat range, and fire closure.

local ADD_ARMS = {
    [ENTRY_THURG] = {
        -- C++ bloodlust (boss_hexlord.cpp:541-549): DoFindFriendlyMissingBuff
        -- (50, 43578) — the first in-combat friendly in 50 yd missing the
        -- bloodlust aura — gets DoCast(43578, false); re-arms 12000
        -- unconditionally.
        { key = "bloodlust", first = 15000, min = 12000, max = 12000,
          fire = function(creature)
              local target = firstMissingBuff(creature, 50.0, SPELL_BLOODLUST)
              if target then
                  creature:CastSpell(target, SPELL_BLOODLUST, false)
              end
          end },
        { key = "cleave", first = 10000, min = 12000, max = 12000,
          fire = function(creature)
              creature:CastSpell(nil, SPELL_CLEAVE_ADD, false)
          end },
    },
    [ENTRY_ALYSON] = {
        -- C++ flashheal arm (boss_hexlord.cpp:623-641): every 2.5s the
        -- most-HP-missing friendly in 99 yd with >30000 missing HP is
        -- flash-healed when within 50 yd (the farther MoveChase leg is
        -- commented-out dead C++ — no-op). With no heal target,
        -- urand(0,1) picks the dispel leg: 1 -> most-missing friendly in
        -- 50 yd, 0 -> random threat-list target (randomThreatTarget —
        -- SelectTarget(Random, 0) with playerOnly=FALSE, UnitAI.h:170, no
        -- range gate).
        { key = "flashheal", first = 2500, min = 2500, max = 2500,
          fire = function(creature)
              local target = lowestHpFriendly(creature, 99.0, 30000)
              if target then
                  if creature:GetDistance(target) <= 50.0 then
                      creature:CastSpell(target, SPELL_FLASH_HEAL, false)
                  end
              elseif math.random(0, 1) == 1 then
                  target = lowestHpFriendly(creature, 50.0, 0)
                  if target then
                      creature:CastSpell(target, SPELL_DISPEL_MAGIC, false)
                  end
              else
                  target = randomThreatTarget(creature)
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
        -- C++ venom spit (boss_hexlord.cpp:885): SelectTarget(Random, 0,
        -- 100, true) — threat-list players alive within 100 yd.
        { key = "venomspit", first = 5000, min = 2500, max = 2500,
          fire = function(creature)
              local target = randomThreatPlayerInRange(creature, 100)
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
    -- Koragg's mighty-blow/cold-stare arms are upstream-dead C++: the
    -- UpdateAI checks (timer <= diff) but never decrements the timers
    -- in an else leg (boss_hexlord.cpp:984-993), so they never fire.
    -- The empty table keeps the registration shape without inventing
    -- casts.
    [ENTRY_KORAGG] = {},
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
    creature:DoZoneInCombat() -- boss_hexlord.cpp:227 (boss_hexlord_addAI::JustEngagedWith sole arm; bridge live 2026-10-23)
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
