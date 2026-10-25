-- Bloodlord Mandokir (Zul'Gurub) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulGurub/boss_mandokir.cpp
-- (boss_mandokirAI + npc_ohganAI + npc_vilebranch_speakerAI);
-- zulgurub.h:36 (DATA_MANDOKIR = 6, optional boss), :60
-- (NPC_MANDOKIR = 11382), :61 (NPC_OHGAN = 14988), :62
-- (NPC_VILEBRANCH_SPEAKER = 11391), :63 (NPC_CHAINED_SPIRT = 15117),
-- :37 (DATA_JINDO = 7), :44 (DATA_VILEBRANCH_SPEAKER = 13).
-- Creature entries: 11382 Bloodlord Mandokir (C++ ScriptName
-- "boss_mandokir" per AddSC_boss_mandokir; zulgurub.h NPC_MANDOKIR);
-- 14988 Ohgan ("npc_ohgan"); 11391 Vilebranch Speaker
-- ("npc_vilebranch_speaker"). The 20 chained spirits 15117 have no C++
-- CreatureScript in this file and are not registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee
-- is engine-driven in Go (creature combat tick), like C++.
-- Fight shape: enter combat -> Talk(SAY_AGGRO=0), arm overpower 24407
-- {7s,9s} then {6s,12s} on the victim (triggered DoCastVictim,
-- C++-exact) / mortal strike 16856 {12s,18s} then {12s,18s} on the
-- victim, cast only when the victim is below 50% health (triggered
-- DoCastVictim with the HealthBelowPct(50) gate, C++-exact) /
-- whirlwind 13736 {24s,30s} then {22s,26s} self-cast (non-triggered
-- DoCast(me), C++-exact) / watch 24314 {13s,15s} then {12s,15s} on a
-- random alive player within 100 yd (Talk(SAY_WATCH=2) whispered to the
-- watched player, C++-exact) / charge 24408 {33s,38s} then {22s,30s} on a random
-- alive player within 40 yd (non-triggered DoCast, C++-exact). Re-arms are
-- unconditional (nil-victim/nil-target ticks cast nothing but keep the
-- schedule, jeklik convention). Every third player kill: Talk
-- (SAY_DING_KILL=1), triggered self-cast 24312 (level up), kill count
-- reset. Ohgan (14988): sunder armor 24317 5s after Reset then
-- {10s,15s} on the victim (triggered DoCastVictim, C++-exact);
-- summoned at engage via the live SummonCreature bridge + DoZoneInCombat
-- tail (arm above).
-- Vilebranch Speaker (11391): demoralizing shout
-- 13730 {2s,4s} after Reset then {22s,30s} self-cast (non-triggered
-- DoCast(me), C++-exact) / cleave 15284 {5s,8s} after Reset then
-- {6s,9s} on the victim (triggered DoCastVictim, C++-exact).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate has
-- no live UNIT_STATE_CASTING read bridge (IsCasting is constant-false — Go casts are fire-and-forget) — timers fire unconditionally (jeklik
-- convention); no instance-script model — boss admission via the
-- luaBossAI shim (_Reset/_JustDied/BossAI::JustEngagedWith, the
-- GetZulGurubAI bookkeeping, and the DATA_MANDOKIR encounter-state
-- arms skipped), so the pre-combat intro machine (Reset's
-- SetImmuneToAll(true) + EVENT_CHECK_START 1s poll of
-- DATA_MANDOKIR == SPECIAL from the speaker's death, MovePoint to
-- PosMandokir[1], EVENT_STARTED -> SetImmuneToAll(false) +
-- MovePath(PATH_MANDOKIR), the waypoint-arrival MovementInform
-- (ev6 IS the waypoint-arrival bridge — creaturemotion.go
-- waypointNodeArrival — but the intro path never launches in Lua,
-- no MovePath bridge) carrying SetHomePosition +
-- MoveTargetedHome + SetBossState(NOT_STARTED) at POINT_MANDOKIR_END,
-- the JustReachedHome SetImmuneToAll(false) arm) has no bearer —
-- Mandokir simply starts combat already at the arena; the Mount
-- (MODEL_OHGAN_MOUNT = 15271) / Dismount arms port via the
-- creature:Mount/Dismount bridge (UNIT_FIELD_MOUNTDISPLAYID +
-- UNIT_FLAG_MOUNT, Unit.cpp:8310/8365); the Ohgan and chained-spirit
-- summon arms ARE ported at engage (live SummonCreature bridge, see
-- below), and SummonedCreatureDies' Ohgan frenzy 24318 +
-- Talk(SAY_OHGAN_DEAD=4) arm ports via the Ohgan Died handler's
-- FindNearestCreature boss relay; the
-- KilledUnit jindo Talk(SAY_GRATS_JINDO) arm rides GetCreaturesInGrid
-- (11380 is unique in Zul'Gurub); the mortal strike HealthBelowPct(50) check
-- uses post-damage GetHealthPct (no pre-damage health bridge);
-- spell_threatening_gaze (the 24314 -> 24315/24316 AuraScript OnRemove
-- chain) is an AuraScript — not modeled (standing gap); the speaker's
-- JustDied SetBossState(DATA_MANDOKIR, SPECIAL) arm has no bearer (no
-- instance-script model).

local ENTRY_MANDOKIR = 11382
local ENTRY_JINDO = 11380 -- NPC_JINDO_THE_HEXXER (zulgurub.h:56)
local ENTRY_OHGAN = 14988
local ENTRY_CHAINED_SPIRT = 15117 -- NPC_CHAINED_SPIRT (zulgurub.h:63)
local ENTRY_SPEAKER = 11391

local SPELL_OVERPOWER = 24407
local SPELL_MORTAL_STRIKE = 16856
local SPELL_WHIRLWIND = 13736
local SPELL_WATCH = 24314
local SPELL_CHARGE = 24408
local SPELL_LEVEL_UP = 24312
local SPELL_SUNDERARMOR = 24317
local SPELL_FRENZY = 24318
local SPELL_DEMORALIZING_SHOUT = 13730
local SPELL_CLEAVE = 15284

local SAY_AGGRO = 0
local SAY_DING_KILL = 1
local SAY_OHGAN_DEAD = 4
local SAY_GRATS_JINDO = 0 -- boss_mandokir.cpp:36
local SAY_WATCH = 2

local timers = {}
local state = {}

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

-- C++ SelectTarget(Random, 0, range, true) (boss_mandokir.cpp:261/
-- :269): random threat-list player alive within range, nil when none
-- qualify.
local function randomThreatPlayerInRange(creature, range)
    local targets = creature:GetAITargets()
    local out = {}
    for _, unit in ipairs(targets) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) <= range then
            out[#out + 1] = unit
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
end

local function mandokirState(guid)
    local st = state[guid]
    if not st then
        st = { killCount = 0 }
        state[guid] = st
    end
    return st
end

-- C++ EVENT_OVERPOWER: triggered DoCastVictim(24407); re-arm {6s,12s}.
local function onOverpower(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_OVERPOWER, true)
    end
    schedule(guid, "overpower", math.random(6000, 12000), function()
        onOverpower(creature, guid)
    end)
end

-- C++ EVENT_MORTAL_STRIKE: victim below 50% health -> triggered
-- DoCastVictim(16856); re-arm {12s,18s} regardless (re-arm is outside
-- the health gate, C++-exact).
local function onMortalStrike(creature, guid)
    local victim = creature:GetVictim()
    -- C++ HealthBelowPct(50) = strict health < trunc(max*50/100); the float
    -- bridge fires 1 HP early on odd maxHealth, so use the floor boundary.
    if victim and victim:GetHealth() < math.floor(victim:GetMaxHealth() * 50 / 100) then
        creature:CastSpell(victim, SPELL_MORTAL_STRIKE, true)
    end
    schedule(guid, "mortalstrike", math.random(12000, 18000), function()
        onMortalStrike(creature, guid)
    end)
end

-- C++ EVENT_WHIRLWIND: non-triggered DoCast(me, 13736); re-arm
-- {22s,26s}.
local function onWhirlwind(creature, guid)
    creature:CastSpell(creature, SPELL_WHIRLWIND)
    schedule(guid, "whirlwind", math.random(22000, 26000), function()
        onWhirlwind(creature, guid)
    end)
end

-- C++ EVENT_WATCH_PLAYER: random 100-yd player -> DoCast(player,
-- SPELL_WATCH) + Talk(SAY_WATCH, player) — the text id goes to the watched
-- player as a whisper target (CreatureAI::Talk(id, whisperTarget),
-- CreatureAI.cpp:48); re-arm {12s,15s}.
local function onWatchPlayer(creature, guid)
    local target = randomThreatPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_WATCH)
        creature:Talk(SAY_WATCH, target)
    end
    schedule(guid, "watch", math.random(12000, 15000), function()
        onWatchPlayer(creature, guid)
    end)
end

-- C++ EVENT_CHARGE_PLAYER: non-triggered DoCast(SelectTarget(Random,
-- 0, 40.0f, true), SPELL_CHARGE); re-arm {22s,30s}. Nil-target ticks
-- cast nothing but keep the schedule.
local function onChargePlayer(creature, guid)
    local target = randomThreatPlayerInRange(creature, 40)
    if target then
        creature:CastSpell(target, SPELL_CHARGE)
    end
    schedule(guid, "charge", math.random(22000, 30000), function()
        onChargePlayer(creature, guid)
    end)
end

local function mandokirResetState(guid)
    cancelTimers(guid)
    state[guid] = nil
end

-- PosSummonChainedSpirits (boss_mandokir.cpp:72-93).
local POS_CHAINED_SPIRITS = {
    { -12167.17, -1979.330, 133.0992, 2.268928 },
    { -12262.74, -1953.394, 133.5496, 0.593412 },
    { -12176.89, -1983.068, 133.7841, 2.129302 },
    { -12226.45, -1977.933, 132.7982, 1.466077 },
    { -12204.74, -1890.431, 135.7569, 4.415683 },
    { -12216.70, -1891.806, 136.3496, 4.677482 },
    { -12236.19, -1892.034, 134.1041, 5.044002 },
    { -12248.24, -1893.424, 134.1182, 5.270895 },
    { -12257.36, -1897.663, 133.1484, 5.462881 },
    { -12265.84, -1903.077, 133.1649, 5.654867 },
    { -12158.69, -1972.707, 133.8751, 2.408554 },
    { -12178.82, -1891.974, 134.1786, 3.944444 },
    { -12193.36, -1890.039, 135.1441, 4.188790 },
    { -12275.59, -1932.845, 134.9017, 0.174533 },
    { -12273.51, -1941.539, 136.1262, 0.314159 },
    { -12247.02, -1963.497, 133.9476, 0.872665 },
    { -12238.68, -1969.574, 133.6273, 1.134464 },
    { -12192.78, -1982.116, 132.6966, 1.919862 },
    { -12210.81, -1979.316, 133.8700, 1.797689 },
    { -12283.51, -1924.839, 133.5170, 0.069813 },
}

local function mandokirEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:SetHomePosition(creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
    creature:Talk(SAY_AGGRO)
    creature:Dismount() -- C++ JustEngagedWith Dismount, Talk then Dismount order.
    -- C++ JustEngagedWith (boss_mandokir.cpp:160-163): SummonCreature
    -- (NPC_OHGAN, x-3, TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT 35s) +
    -- 20x SummonCreature(NPC_CHAINED_SPIRT, TEMPSUMMON_CORPSE_DESPAWN) —
    -- summon bridge (lua_creature_events.go:877); Ohgan's AI is registered
    -- above, so the summon fights; the spirits have no C++ AI either.
    creature:SummonCreature(ENTRY_OHGAN, creature:GetX() - 3, creature:GetY(), creature:GetZ(), creature:GetO(), 3, 35000)
    for _, pos in ipairs(POS_CHAINED_SPIRITS) do
        creature:SummonCreature(ENTRY_CHAINED_SPIRT, pos[1], pos[2], pos[3], pos[4], 4)
    end
    creature:DoZoneInCombat() -- C++ JustEngagedWith tail: engage every
    -- player on the dungeon map (DoZoneInCombat bridge live 2026-10-23).
    schedule(guid, "overpower", math.random(7000, 9000), function()
        onOverpower(creature, guid)
    end)
    schedule(guid, "mortalstrike", math.random(12000, 18000), function()
        onMortalStrike(creature, guid)
    end)
    schedule(guid, "whirlwind", math.random(24000, 30000), function()
        onWhirlwind(creature, guid)
    end)
    schedule(guid, "watch", math.random(13000, 15000), function()
        onWatchPlayer(creature, guid)
    end)
    schedule(guid, "charge", math.random(33000, 38000), function()
        onChargePlayer(creature, guid)
    end)
end

-- C++ KilledUnit: player victims only; every third kill -> Talk
-- (SAY_DING_KILL), jindo congrats via entry search (no instance-script
-- bridge), triggered self-cast 24312, count reset.
local function mandokirTargetDied(event, creature, victim)
    if victim:GetObjectType() ~= "Player" then
        return
    end
    local st = mandokirState(creature:GetGUID())
    st.killCount = st.killCount + 1
    if st.killCount >= 3 then
        creature:Talk(SAY_DING_KILL)
        -- C++ KilledUnit (boss_mandokir.cpp:173-176): instance->GetCreature
        -- (DATA_JINDO) approximated by entry search — NPC_JINDO_THE_HEXXER
        -- (11380) is unique in Zul'Gurub, so any match is Jin'do.
        for _, jindo in ipairs(creature:GetCreaturesInGrid(ENTRY_JINDO, 5000)) do
            if jindo:IsAlive() then
                jindo:Talk(SAY_GRATS_JINDO)
            end
        end
        creature:CastSpell(creature, SPELL_LEVEL_UP, true)
        st.killCount = 0
    end
end

local function mandokirLeaveCombat(event, creature)
    mandokirResetState(creature:GetGUID())
end

local function mandokirDied(event, creature, killer)
    mandokirResetState(creature:GetGUID())
end

local function mandokirReset(event, creature)
    -- C++ Reset's Z>140 intro/summon arms have no bridge (no movement,
    -- instance-script, or summon model) — only the timer cancel, the
    -- Z-gated SetImmuneToAll(true) (SetFlag 59, 0x300, in C++ order
    -- after Initialize) and the Reset-tail Mount(MODEL_OHGAN_MOUNT
    -- 15271) are modeled. The EVENT_STARTED clear rides the unported
    -- intro machine; the JustReachedHome clear is ported below.
    mandokirResetState(creature:GetGUID())
    if creature:GetZ() > 140.0 then
        creature:SetFlag(59, 0x300) -- IMMUNE_TO_PC|IMMUNE_TO_NPC
    end
    creature:Mount(15271)
end

-- C++ JustReachedHome: me->SetImmuneToAll(false) only.
local function mandokirReachHome(event, creature)
    creature:RemoveFlag(59, 0x300)
end

RegisterCreatureEvent(ENTRY_MANDOKIR, 1, mandokirEnterCombat)
RegisterCreatureEvent(ENTRY_MANDOKIR, 2, mandokirLeaveCombat)
RegisterCreatureEvent(ENTRY_MANDOKIR, 3, mandokirTargetDied)
RegisterCreatureEvent(ENTRY_MANDOKIR, 4, mandokirDied)
RegisterCreatureEvent(ENTRY_MANDOKIR, 23, mandokirReset)
RegisterCreatureEvent(ENTRY_MANDOKIR, 24, mandokirReachHome)

-- Ohgan (npc_ohgan, C++ ScriptName for 14988): sunder armor 24317 5s
-- after Reset then {10s,15s} on the victim (triggered DoCastVictim,
-- C++-exact); melee is engine-driven. Summoned at engage by the
-- mandokir arm above (live SummonCreature bridge).
local function onSunderArmor(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SUNDERARMOR, true)
    end
    schedule(guid, "sunder", math.random(10000, 15000), function()
        onSunderArmor(creature, guid)
    end)
end

local function ohganEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "sunder", 5000, function()
        onSunderArmor(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_OHGAN, 1, ohganEnterCombat)
RegisterCreatureEvent(ENTRY_OHGAN, 2, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_OHGAN, 4, function(event, creature, killer)
    cancelTimers(creature:GetGUID())
    -- C++ SummonedCreatureDies (boss_mandokir.cpp:184-190): frenzy
    -- self-cast + Talk on the boss; relayed via FindNearestCreature
    -- (garaxxas/delrissa liveness twin — the boss object is not
    -- reachable from the summon's own Died handler otherwise).
    -- Temp-summon expiry is UnSummon, not death, so this fires only on
    -- a real kill; a missing boss (already dead/gone) skips silently.
    local boss = creature:FindNearestCreature(ENTRY_MANDOKIR, 100.0)
    if boss then
        boss:CastSpell(boss, SPELL_FRENZY)
        boss:Talk(SAY_OHGAN_DEAD)
    end
end)
RegisterCreatureEvent(ENTRY_OHGAN, 23, function(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "sunder", 5000, function()
        onSunderArmor(creature, guid)
    end)
end)

-- Vilebranch Speaker (npc_vilebranch_speaker, C++ ScriptName for
-- 11391): demoralizing shout 13730 {2s,4s} after Reset then {22s,30s}
-- self-cast (non-triggered DoCast(me), C++-exact) / cleave 15284 {5s,8s}
-- after Reset then {6s,9s} on the victim (triggered DoCastVictim,
-- C++-exact); melee is engine-driven. The JustDied
-- SetBossState(DATA_MANDOKIR, SPECIAL) arm has no bearer (no
-- instance-script model) — not modeled.
local function onDemoralizingShout(creature, guid)
    creature:CastSpell(creature, SPELL_DEMORALIZING_SHOUT)
    schedule(guid, "shout", math.random(22000, 30000), function()
        onDemoralizingShout(creature, guid)
    end)
end

local function onSpeakerCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE, true)
    end
    schedule(guid, "cleave", math.random(6000, 9000), function()
        onSpeakerCleave(creature, guid)
    end)
end

-- C++ npc_vilebranch_speakerAI: the timers are armed in Initialize/Reset
-- ({2s,4s} shout, {5s,8s} cleave) and there is NO enter-combat re-arm —
-- UpdateAI's UpdateVictim gate means they effectively start ticking at
-- combat start with the initial values. The Lua enter-combat arm therefore
-- re-arms with the Reset values, not the re-arm ranges.
local function speakerArmTimers(creature, guid)
    schedule(guid, "shout", math.random(2000, 4000), function()
        onDemoralizingShout(creature, guid)
    end)
    schedule(guid, "cleave", math.random(5000, 8000), function()
        onSpeakerCleave(creature, guid)
    end)
end

local function speakerReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    speakerArmTimers(creature, guid)
end

RegisterCreatureEvent(ENTRY_SPEAKER, 1, function(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    speakerArmTimers(creature, guid)
end)
RegisterCreatureEvent(ENTRY_SPEAKER, 2, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_SPEAKER, 4, function(event, creature, killer)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_SPEAKER, 23, speakerReset)
