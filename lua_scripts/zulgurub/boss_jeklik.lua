-- High Priestess Jeklik (Zul'Gurub) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulGurub/boss_jeklik.cpp
-- (boss_jeklikAI + npc_batriderAI); zulgurub.h:30 (DATA_JEKLIK = 0).
-- Creature entries: 14517 Jeklik (C++ ScriptName "boss_jeklik" per
-- AddSC_boss_jeklik; wowhead npc=14517/high-priestess-jeklik); 14965
-- frenzied bat (C++ ScriptName "npc_batrider" — the phase-2 flying bomb
-- bat). The bloodseeker bat 11368 summoned in phase one has no C++
-- CreatureScript in this file and is not registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied, 9
-- OnDamaged (pre-damage), 23 OnReset. Timers via CreateLuaEvent; melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Fight shape (C++ PHASE_ONE=1 bat form / PHASE_TWO=2 troll form):
--   enter combat: Talk SAY_AGGRO(0), self-cast 23966 (bat form);
--     charge 22911 at 20s then {15s,30s} on a random alive player (C++
--     DoCast on a random player, then AttackStart — the AttackStart arm
--     has no movement-model bridge and is skipped); sonic burst 23918
--     at 8s then {8s,13s} on the victim; screech 6605 at 13s then
--     {18s,26s} on the victim; spawn six bloodseeker bats 11368 at the
--     fixed SpawnBat positions at 60s then every 60s (no summon model —
--     the bats never exist, so the timer arm keeps only the re-arm).
--   phase two: the event-9 hook runs when health is at <=50% while in
--     phase one (C++ DamageTaken fires pre-damage and HealthAbovePct(50)
--     evaluates current health with no damage projection; the arm fires
--     when !(health > floor(maxHealth*50/100))): RemoveAura(23966), the four
--     phase-1 timers are canceled (C++ SetPhase(PHASE_TWO) drops the
--     phase-1 EventMap entries C++-exact), and the phase-2 timers arm:
--     shadow word pain 23952 at 6s then {12s,18s} on a random alive
--     player; mind flay 23953 at 11s then 16s on the victim; chain mind
--     flay 26044 at 26s then {15s,30s} on the victim (the C++ comment
--     says the ID is unknown/disabled, but the code schedules and casts
--     it — modeled C++-exact); greater heal 23954 at 50s then {25s,35s}
--     self-cast; spawn flying bat 14965 at 10s then {10s,15s} (summon
--     bridge, 2026-10-22 — the bat spawns at the target's position +15z;
--     its AttackStart leg has no bridge, so it idles until the engine
--     aggros it; the batrider AI below is registered so a spawned 14965
--     fights correctly once engaged). The
--     InterruptNonMeleeSpells(false) arms before the chain-mind-flay and
--     greater-heal casts have no bridge (no UNIT_STATE model — timers
--     fire unconditionally, malchezaar convention).
--   died: Talk SAY_DEATH(2), cancel timers.
-- Batrider (14965): first bomb 40332 lands 2s after entering combat
-- (C++ Initialize arms the 2s timer; it only counts down once the
-- UpdateAI victim gate passes), then every 5s on a random alive player;
-- a nil-target tick retries in 1s (C++ leaves the timer armed and
-- retries on the next UpdateAI tick). The Reset SetFlag
-- NOT_SELECTABLE arm is ported (2026-10-20) via
-- creature:SetFlag(UNIT_FIELD_FLAGS, ...).
-- Deviations from C++: SAY_RAIN_FIRE(1) is defined in the C++ Says enum
-- but never used — not modeled; the six bloodseeker bats (11368) and the
-- fixed SpawnBat coordinates never exist (no bearer); the per-10s
-- frenzied bat (14965) summons ARE ported (2026-10-22 summon bridge); no instance-script model — boss admission
-- via the luaBossAI shim (_Reset/_JustDied/BossAI::JustEngagedWith and
-- the GetZulGurubAI bookkeeping arms skipped); no threat model — the
-- charge and shadow-word-pain targets are random alive players with no
-- victim fallback; ResetThreatList PORTED 2026-10-19
-- (creature:ClearThreatList, bridged lua_creature_events.go); no movement
-- model — the
-- charge AttackStart arm skipped; SetCanFly IS bridged
-- (lua_creature_events.go, PORTED 2026-10-22) — the phase-1
-- flight is cosmetic; the C++ while-loop casting gate
-- (UNIT_STATE_CASTING checks in UpdateAI) has no bridge — timers fire
-- unconditionally.

local ENTRY_JEKLIK = 14517
local ENTRY_FRENZIED_BAT = 14965

local SAY_AGGRO = 0
local SAY_DEATH = 2

local SPELL_CHARGE = 22911
local SPELL_SONIC_BURST = 23918
local SPELL_SCREECH = 6605
local SPELL_SHADOW_WORD_PAIN = 23952
local SPELL_MIND_FLAY = 23953
local SPELL_CHAIN_MIND_FLAY = 26044
local SPELL_GREATER_HEAL = 23954
local SPELL_BAT_FORM = 23966
local SPELL_BOMB = 40332

-- UnitDefines.h:149; SetFlag/RemoveFlag on UNIT_FIELD_FLAGS (59) is
-- bridged (engine/world/lua_creature_events.go:968).
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local PHASE_ONE = 1
local PHASE_TWO = 2

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

local function jeklikState(guid)
    local st = state[guid]
    if not st then
        st = { phase = PHASE_ONE }
        state[guid] = st
    end
    return st
end

local function alivePlayers(creature)
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

local function randomPlayer(creature)
    local candidates = alivePlayers(creature)
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_CHARGE_JEKLIK: non-triggered 22911 on a random player
-- (no-target tick keeps the re-arm, C++-exact); re-arm {15s,30s}.
local function onCharge(creature, guid)
    if jeklikState(guid).phase ~= PHASE_ONE then
        return
    end
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_CHARGE)
    end
    schedule(guid, "charge", math.random(15000, 30000), function()
        onCharge(creature, guid)
    end)
end

-- C++ EVENT_SONIC_BURST: DoCastVictim(23918); re-arm {8s,13s}.
local function onSonicBurst(creature, guid)
    if jeklikState(guid).phase ~= PHASE_ONE then
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SONIC_BURST)
    end
    schedule(guid, "sonic", math.random(8000, 13000), function()
        onSonicBurst(creature, guid)
    end)
end

-- C++ EVENT_SCREECH: DoCastVictim(6605); re-arm {18s,26s}.
local function onScreech(creature, guid)
    if jeklikState(guid).phase ~= PHASE_ONE then
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SCREECH)
    end
    schedule(guid, "screech", math.random(18000, 26000), function()
        onScreech(creature, guid)
    end)
end

-- C++ EVENT_SPAWN_BATS: the six 11368 summons have no bearer — only
-- the 60s re-arm is modeled (one-shot per cycle, C++-exact).
local function onSpawnBats(creature, guid)
    if jeklikState(guid).phase ~= PHASE_ONE then
        return
    end
    schedule(guid, "spawnbats", 60000, function()
        onSpawnBats(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_WORD_PAIN: DoCast(23952) on a random player
-- (no-target tick keeps the re-arm); re-arm {12s,18s}.
local function onShadowWordPain(creature, guid)
    if jeklikState(guid).phase ~= PHASE_TWO then
        return
    end
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_WORD_PAIN)
    end
    schedule(guid, "swp", math.random(12000, 18000), function()
        onShadowWordPain(creature, guid)
    end)
end

-- C++ EVENT_MIND_FLAY: DoCastVictim(23953); re-arm 16s.
local function onMindFlay(creature, guid)
    if jeklikState(guid).phase ~= PHASE_TWO then
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MIND_FLAY)
    end
    schedule(guid, "mindflay", 16000, function()
        onMindFlay(creature, guid)
    end)
end

-- C++ EVENT_CHAIN_MIND_FLAY: InterruptNonMeleeSpells (no bridge) +
-- DoCastVictim(26044); re-arm {15s,30s}.
local function onChainMindFlay(creature, guid)
    if jeklikState(guid).phase ~= PHASE_TWO then
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CHAIN_MIND_FLAY)
    end
    schedule(guid, "chainflay", math.random(15000, 30000), function()
        onChainMindFlay(creature, guid)
    end)
end

-- C++ EVENT_GREATER_HEAL: InterruptNonMeleeSpells (no bridge) +
-- DoCast(me, 23954); re-arm {25s,35s}.
local function onGreaterHeal(creature, guid)
    if jeklikState(guid).phase ~= PHASE_TWO then
        return
    end
    creature:CastSpell(creature, SPELL_GREATER_HEAL)
    schedule(guid, "heal", math.random(25000, 35000), function()
        onGreaterHeal(creature, guid)
    end)
end

-- C++ EVENT_SPAWN_FLYING_BATS (boss_jeklik.cpp:185-190): pick a random
-- alive player (SelectTarget Random, 0, 0.f, playerOnly TRUE — no range
-- cap) and summon a frenzied bat (14965) at the target's position +15z,
-- o = 0 -> my orientation (C++ zero-o fallback, Object.cpp:2061),
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT (3), 15s = 15000ms.
-- flyingBat->AI()->AttackStart(target) has no bridge — the batrider AI
-- below is registered, so the bat fights once engaged (documented gap).
-- Re-arm {10s,15s} (one-shot per cycle, C++-exact).
local function onSpawnFlyingBats(creature, guid)
    if jeklikState(guid).phase ~= PHASE_TWO then
        return
    end
    local target = randomPlayer(creature)
    if target then
        creature:SummonCreature(ENTRY_FRENZIED_BAT, target:GetX(),
            target:GetY(), target:GetZ() + 15.0, creature:GetO(), 3, 15000)
    end
    schedule(guid, "flyingbat", math.random(10000, 15000), function()
        onSpawnFlyingBats(creature, guid)
    end)
end

-- Phase-1 timer keys canceled when entering phase two (C++ SetPhase
-- drops the phase-1 EventMap entries C++-exact).
local PHASE_ONE_TIMER_KEYS = { "charge", "sonic", "screech", "spawnbats" }

local function jeklikResetState(guid)
    cancelTimers(guid)
    state[guid] = nil
end

local function jeklikEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    state[guid] = { phase = PHASE_ONE }
    creature:Talk(SAY_AGGRO)
    -- C++ :110: me->SetCanFly(true) before the bat-form DoCast
    -- (bridge live, lua_creature_events.go).
    creature:SetCanFly(true)
    creature:CastSpell(creature, SPELL_BAT_FORM)
    schedule(guid, "charge", 20000, function()
        onCharge(creature, guid)
    end)
    schedule(guid, "sonic", 8000, function()
        onSonicBurst(creature, guid)
    end)
    schedule(guid, "screech", 13000, function()
        onScreech(creature, guid)
    end)
    schedule(guid, "spawnbats", 60000, function()
        onSpawnBats(creature, guid)
    end)
end

-- C++ DamageTaken: phase one and !HealthAbovePct(50) -> drop the bat
-- form and arm the phase-two machine. DamageTaken fires pre-damage
-- (Unit.cpp DealDamage hooks before the health reduction) and C++
-- HealthAbovePct(50) is health > CountPctFromMaxHealth(50) on current
-- health -- no damage projection, so the arm fires when the health is
-- already at or under the floored threshold (Unit.h:874).
local function jeklikDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = jeklikState(guid)
    if st.phase ~= PHASE_ONE then
        return
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if health > math.floor(maxHealth * 50 / 100) then
        return
    end
    st.phase = PHASE_TWO
    creature:RemoveAura(SPELL_BAT_FORM)
    -- C++ DamageTaken phase-1->2 (:116-119): RemoveAurasDueToSpell, then
    -- me->SetCanFly(false), then ResetThreatList (bridged
    -- lua_creature_events.go) — C++ arm order kept.
    creature:SetCanFly(false)
    creature:ClearThreatList()
    for _, key in ipairs(PHASE_ONE_TIMER_KEYS) do
        local per = timers[guid]
        if per and per[key] then
            RemoveEventById(per[key])
            per[key] = nil
        end
    end
    schedule(guid, "swp", 6000, function()
        onShadowWordPain(creature, guid)
    end)
    schedule(guid, "mindflay", 11000, function()
        onMindFlay(creature, guid)
    end)
    schedule(guid, "chainflay", 26000, function()
        onChainMindFlay(creature, guid)
    end)
    schedule(guid, "heal", 50000, function()
        onGreaterHeal(creature, guid)
    end)
    schedule(guid, "flyingbat", 10000, function()
        onSpawnFlyingBats(creature, guid)
    end)
end

local function jeklikLeaveCombat(event, creature)
    jeklikResetState(creature:GetGUID())
end

local function jeklikDied(event, creature, killer)
    jeklikResetState(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function jeklikReset(event, creature)
    jeklikResetState(creature:GetGUID())
    -- C++ Reset (:88-91) is _Reset() only — no SetCanFly leg exists
    -- (the old "SetCanFly restore / no bridge" note was stale twice
    -- over: the bridge exists and the C++ arm never calls it).
end

RegisterCreatureEvent(ENTRY_JEKLIK, 1, jeklikEnterCombat)
RegisterCreatureEvent(ENTRY_JEKLIK, 2, jeklikLeaveCombat)
RegisterCreatureEvent(ENTRY_JEKLIK, 4, jeklikDied)
RegisterCreatureEvent(ENTRY_JEKLIK, 9, jeklikDamageTaken)
RegisterCreatureEvent(ENTRY_JEKLIK, 23, jeklikReset)

-- npc_batrider (14965, frenzied bat): summoned every 10-15s in phase two
-- (summon bridge, 2026-10-22); registered so each spawned 14965 drops
-- bombs once engaged.
local batTimers = {}

local function batriderBomb(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_BOMB)
        batTimers[guid] = CreateLuaEvent(function()
            batriderBomb(creature, guid)
        end, 5000)
    else
        -- C++ leaves _bombTimer armed (expired) and retries on the next
        -- UpdateAI tick: re-arm at 1ms so the next pump tick retries.
        batTimers[guid] = CreateLuaEvent(function()
            batriderBomb(creature, guid)
        end, 1)
    end
end

local function batriderResetState(guid)
    local id = batTimers[guid]
    if id then
        RemoveEventById(id)
        batTimers[guid] = nil
    end
end

local function batriderEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    batriderResetState(guid)
    batTimers[guid] = CreateLuaEvent(function()
        batriderBomb(creature, guid)
    end, 2000)
end

local function batriderLeaveCombat(event, creature)
    batriderResetState(creature:GetGUID())
end

local function batriderDied(event, creature, killer)
    batriderResetState(creature:GetGUID())
end

local function batriderReset(event, creature)
    -- C++ npc_batriderAI::Reset (boss_jeklik.cpp:228-231): Initialize()
    -- arms the 2s bomb initializer + SetFlag(UNIT_FIELD_FLAGS,
    -- NOT_SELECTABLE) (ported 2026-10-20); no RemoveFlag arm exists in
    -- the C++ file, so Reset re-asserts on evade — skip on the death
    -- path, where C++ Reset never runs. Out of combat the timer is
    -- moot, so Reset only cancels (the enter-combat arm carries the
    -- initializer, C++-exact).
    if not creature:IsDead() then
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    end
    batriderResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_FRENZIED_BAT, 1, batriderEnterCombat)
RegisterCreatureEvent(ENTRY_FRENZIED_BAT, 2, batriderLeaveCombat)
RegisterCreatureEvent(ENTRY_FRENZIED_BAT, 4, batriderDied)
RegisterCreatureEvent(ENTRY_FRENZIED_BAT, 23, batriderReset)
