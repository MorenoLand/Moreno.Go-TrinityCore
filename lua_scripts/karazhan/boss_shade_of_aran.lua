-- Shade of Aran (Karazhan) — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/boss_shade_of_aran.cpp
-- Creature entry: 16524 (wowhead wotlk npc=16524/shade-of-aran).
-- Instance data: DATA_ARAN = 6, DATA_GO_LIBRARY_DOOR = 21
-- (karazhan.h:36,49); the encounter never reads encounter state — the
-- library-door arms have no Go model (see deviations).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape: every 1s a random normal bolt (frostbolt 29954 / fireball
-- 29953 / arc missile 29955 — the three 5s school cooldowns are only ever
-- armed by the interrupt arm, which has no Lua-modelable input, so all
-- three are always available); every 5-20s a secondary (AOE counterspell
-- 29961 or chains of ice 29991); every 35-40s a super spell that never
-- repeats the previous one — arcane explosion (blink center 29967,
-- player pull 32265, mass slow 30035, then 29973), flame wreath (29946 on
-- up to 3 random players; a 500ms watcher punishes movement beyond 3 yd
-- from the cast point), or circular blizzard. Below 40% health once, four
-- water elementals; at 12 minutes and every 60s after, shadow-of-Aran
-- berserk (Talk only). The drink/potion/pyroblast mana sequence needs
-- power APIs the creature Lua object does not expose (see deviations).
-- Deviations from C++: no summon model — the 4 water elementals (17167),
-- the 5 shadow-of-Aran summons (18254), and the blizzard creature (17161)
-- never spawn; the elemental sub-40% arm and the 12-min berserk keep their
-- Talk/refresh structure with the SummonCreature calls skipped (blizzard
-- is approximated by casting 29951 on self, like the C++ aura the summoned
-- creature would cast on itself). The water elemental AI itself
-- ("npc_aran_elemental") is registered for entry 17167 below so elementals
-- spawned by other means cast waterbolt 31012 every 2-5s. No
-- instance-script model — the library-door open/close arms and the
-- DATA_ARAN SetBossState calls are skipped (admission-only via the
-- luaBossAI shim in engine/world/boss_ai.go). No threat model — the
-- flame-wreath target pool is random alive players in the instance rather
-- than the threat list. No UNIT_STATE_CASTING model — timers fire
-- unconditionally (same convention as the other Karazhan ports), and
-- InterruptNonMeleeSpells gates are skipped. No power API on the creature
-- Lua object (no GetPower/SetPower/SetStandState) and no mana drain from
-- visual casts — the whole drink arm (SAY_DRINK=4, mass poly 29963,
-- conjure 29975, drink 29975 30024, potion 32453, AOE pyroblast 29978, the
-- drink-interrupt damage hook, the 10s interrupt timer) is skipped; SAY_4
-- never fires. The SpellHit interrupt arm has no modelable input — the
-- engine's OnHitBySpell hook passes only the spell id, while C++ checks
-- SPELL_EFFECT_INTERRUPT_CAST on the spell effects. No facing on the
-- bridge, so the flame-wreath 3-yd check is 2D distance from the cast
-- point; the C++ punishment is the moved unit casting 20476 on itself
-- with Aran as original caster — the bridge's CastSpell lives on the boss
-- object, so Aran casts 20476 onto the moved player instead (11027 via
-- player:AddAura, as C++ casts it triggered on the unit).

local ENTRY_SHADE_OF_ARAN = 16524
local ENTRY_WATER_ELEMENTAL = 17167

local SAY_AGGRO = 0
local SAY_FLAMEWREATH = 1
local SAY_BLIZZARD = 2
local SAY_EXPLOSION = 3
local SAY_ELEMENTALS = 5
local SAY_KILL = 6
local SAY_TIMEOVER = 7
local SAY_DEATH = 8

local SPELL_FROSTBOLT = 29954
local SPELL_FIREBALL = 29953
local SPELL_ARCMISSLE = 29955
local SPELL_CHAINSOFICE = 29991
local SPELL_MASSSLOW = 30035
local SPELL_FLAME_WREATH = 29946
local SPELL_AOE_CS = 29961
local SPELL_PLAYERPULL = 32265
local SPELL_AEXPLOSION = 29973
local SPELL_BLINK_CENTER = 29967
local SPELL_CIRCULAR_BLIZZARD = 29951
local SPELL_WATERBOLT = 31012
local SPELL_FLAMEWREATH_PUNISH = 20476
local SPELL_FLAMEWREATH_DOT = 11027

local NORMAL_SPELLS = { SPELL_ARCMISSLE, SPELL_FIREBALL, SPELL_FROSTBOLT }

local SUPER_FLAME = 0
local SUPER_BLIZZARD = 1
local SUPER_AE = 2
-- C++: the next super spell is drawn from the two that did not fire last.
local SUPER_FOLLOW = {
    [SUPER_FLAME] = { SUPER_BLIZZARD, SUPER_AE },
    [SUPER_BLIZZARD] = { SUPER_FLAME, SUPER_AE },
    [SUPER_AE] = { SUPER_FLAME, SUPER_BLIZZARD },
}

local timers = {}
local aranState = {}
local elementalTimers = {}

local function cancelTimers(store, guid)
    local per = store[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        store[guid] = nil
    end
end

local function schedule(store, guid, key, delay, fn)
    local per = store[guid]
    if not per then
        per = {}
        store[guid] = per
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function alivePlayersInRange(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist then
            found[#found + 1] = p
        end
    end
    return found
end

local function randomPlayer(creature, maxDist)
    local candidates = alivePlayersInRange(creature, maxDist)
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function dist2d(xa, ya, xb, yb)
    local dx, dy = xa - xb, ya - yb
    return math.sqrt(dx * dx + dy * dy)
end

local function findPlayerByGuid(guid)
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetGUID() == guid and not p:IsDead() then
            return p
        end
    end
    return nil
end

local onNormalCast, onSecondaryCast, onSuperCast, onBerserk, onFlameWreathCheck

local function aranInitState(guid)
    -- C++ Initialize: LastSuperSpell = rand32() % 3; the school cooldowns
    -- stay 0 without the interrupt arm; drink state omitted (no power
    -- bridge); elementalsSpawned per encounter.
    aranState[guid] = {
        lastSuper = math.random(3) - 1,
        elementalsSpawned = false,
        fwRemaining = 0,
        fwTargets = { nil, nil, nil },
    }
end

local function clearFlameWreath(guid)
    local st = aranState[guid]
    if st then
        st.fwRemaining = 0
        st.fwTargets = { nil, nil, nil }
    end
    local per = timers[guid]
    if per and per["fwcheck"] then
        RemoveEventById(per["fwcheck"])
        per["fwcheck"] = nil
    end
end

local function startFlameWreath(creature, guid)
    local st = aranState[guid]
    -- C++ FlameWreathEffect: up to 3 random alive players from the threat
    -- list (random removal while > 3); no threat model on the bridge, so
    -- the pool is the alive instance players.
    local pool = alivePlayersInRange(creature, 100)
    while #pool > 3 do
        table.remove(pool, math.random(#pool))
    end
    local slots = { nil, nil, nil }
    for i, p in ipairs(pool) do
        slots[i] = { guid = p:GetGUID(), x = p:GetX(), y = p:GetY() }
        creature:CastSpell(p, SPELL_FLAME_WREATH, true)
    end
    st.fwTargets = slots
    st.fwRemaining = 20000
    schedule(timers, guid, "fwcheck", 500, function()
        onFlameWreathCheck(creature, guid)
    end)
end

onFlameWreathCheck = function(creature, guid)
    local st = aranState[guid]
    if not st then
        return
    end
    st.fwRemaining = st.fwRemaining - 500
    for i = 1, 3 do
        local slot = st.fwTargets[i]
        if slot then
            local unit = findPlayerByGuid(slot.guid)
            if unit and dist2d(unit:GetX(), unit:GetY(), slot.x, slot.y) > 3 then
                creature:CastSpell(unit, SPELL_FLAMEWREATH_PUNISH)
                unit:AddAura(SPELL_FLAMEWREATH_DOT)
                st.fwTargets[i] = nil
            end
        end
    end
    if st.fwRemaining > 0 then
        schedule(timers, guid, "fwcheck", 500, function()
            onFlameWreathCheck(creature, guid)
        end)
    else
        clearFlameWreath(guid)
    end
end

onNormalCast = function(creature, guid)
    local st = aranState[guid]
    if st then
        -- C++: SelectTarget Random 100 yd; random school from those not on
        -- cooldown (all three here — the interrupt arm has no Lua input).
        local target = randomPlayer(creature, 100)
        if target then
            creature:CastSpell(target, NORMAL_SPELLS[math.random(#NORMAL_SPELLS)])
        end
        if not st.elementalsSpawned and creature:GetHealthPct() < 40 then
            st.elementalsSpawned = true
            -- C++ summons 4 water elementals (17167) on the victim; no
            -- summon model, so only the announcement survives.
            creature:Talk(SAY_ELEMENTALS)
        end
    end
    schedule(timers, guid, "normal", 1000, function()
        onNormalCast(creature, guid)
    end)
end

onSecondaryCast = function(creature, guid)
    if math.random(2) == 1 then
        creature:CastSpell(creature, SPELL_AOE_CS)
    else
        local target = randomPlayer(creature, 100)
        if target then
            creature:CastSpell(target, SPELL_CHAINSOFICE)
        end
    end
    schedule(timers, guid, "secondary", math.random(5000, 20000), function()
        onSecondaryCast(creature, guid)
    end)
end

onSuperCast = function(creature, guid)
    local st = aranState[guid]
    if not st then
        return
    end
    local follow = SUPER_FOLLOW[st.lastSuper]
    st.lastSuper = follow[math.random(2)]
    if st.lastSuper == SUPER_AE then
        creature:Talk(SAY_EXPLOSION)
        creature:CastSpell(creature, SPELL_BLINK_CENTER, true)
        creature:CastSpell(creature, SPELL_PLAYERPULL, true)
        creature:CastSpell(creature, SPELL_MASSSLOW, true)
        creature:CastSpell(creature, SPELL_AEXPLOSION)
    elseif st.lastSuper == SUPER_FLAME then
        creature:Talk(SAY_FLAMEWREATH)
        clearFlameWreath(guid)
        startFlameWreath(creature, guid)
    else
        creature:Talk(SAY_BLIZZARD)
        -- C++ summons the blizzard creature (17161), which casts the
        -- circular blizzard on itself; no summon model, so Aran casts the
        -- same visual on himself.
        creature:CastSpell(creature, SPELL_CIRCULAR_BLIZZARD)
    end
    schedule(timers, guid, "super", math.random(35000, 40000), function()
        onSuperCast(creature, guid)
    end)
end

onBerserk = function(creature, guid)
    -- C++ summons 5 shadow-of-Aran (18254) on the victim; no summon model —
    -- only the announcement survives.
    creature:Talk(SAY_TIMEOVER)
    schedule(timers, guid, "berserk", 60000, function()
        onBerserk(creature, guid)
    end)
end

local function aranEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    aranInitState(guid)
    -- C++ JustEngagedWith: Talk(SAY_AGGRO); the library-door close and
    -- SetBossState(IN_PROGRESS) have no instance-script model.
    creature:Talk(SAY_AGGRO)
    schedule(timers, guid, "normal", 1, function()
        onNormalCast(creature, guid)
    end)
    schedule(timers, guid, "secondary", 5000, function()
        onSecondaryCast(creature, guid)
    end)
    schedule(timers, guid, "super", 35000, function()
        onSuperCast(creature, guid)
    end)
    schedule(timers, guid, "berserk", 720000, function()
        onBerserk(creature, guid)
    end)
end

local function aranLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    aranInitState(guid)
end

local function aranDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    -- C++ JustDied: Talk(SAY_DEATH); the door re-open and SetBossState(DONE)
    -- have no instance-script model.
    creature:Talk(SAY_DEATH)
end

local function aranReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    aranInitState(guid)
end

RegisterCreatureEvent(ENTRY_SHADE_OF_ARAN, 1, aranEnterCombat)
RegisterCreatureEvent(ENTRY_SHADE_OF_ARAN, 2, aranLeaveCombat)
RegisterCreatureEvent(ENTRY_SHADE_OF_ARAN, 3, function(event, creature, victim)
    -- C++ KilledUnit: Talk(SAY_KILL), no target gate.
    creature:Talk(SAY_KILL)
end)
RegisterCreatureEvent(ENTRY_SHADE_OF_ARAN, 4, aranDied)
RegisterCreatureEvent(ENTRY_SHADE_OF_ARAN, 23, aranReset)

-- Water elemental (npc_aran_elemental, C++ ScriptName for 17167): casts
-- waterbolt 31012 on its victim every 2-5s. The elementals themselves are
-- never summoned (no summon model), but the AI is registered so any that
-- exist fight correctly.
local onWaterbolt

local function elementalEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(elementalTimers, guid)
    -- C++ water_elementalAI Reset: CastTimer = 2000 + rand32() % 3000;
    -- combat re-arms at urand(2000, 5000).
    schedule(elementalTimers, guid, "waterbolt", math.random(2000, 5000), function()
        onWaterbolt(creature, guid)
    end)
end

onWaterbolt = function(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_WATERBOLT)
    end
    schedule(elementalTimers, guid, "waterbolt", math.random(2000, 5000), function()
        onWaterbolt(creature, guid)
    end)
end

local function elementalLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(elementalTimers, guid)
end

RegisterCreatureEvent(ENTRY_WATER_ELEMENTAL, 1, elementalEnterCombat)
RegisterCreatureEvent(ENTRY_WATER_ELEMENTAL, 2, elementalLeaveCombat)
RegisterCreatureEvent(ENTRY_WATER_ELEMENTAL, 23, elementalLeaveCombat)
