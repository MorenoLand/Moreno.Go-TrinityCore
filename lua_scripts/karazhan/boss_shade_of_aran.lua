-- Shade of Aran (Karazhan) — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/boss_shade_of_aran.cpp
-- Creature entry: 16524 (wowhead wotlk npc=16524/shade-of-aran).
-- Instance data: DATA_ARAN = 6, DATA_GO_LIBRARY_DOOR = 21
-- (karazhan.h:36,49); the encounter never reads encounter state — the
-- library-door arms port via FindNearestGameObject (184517) + SetGoState.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent. C++ Aran melees only
-- when all three school cooldowns are armed — i.e. only right after a
-- successful interrupt (if (ArcaneCooldown && FireCooldown && FrostCooldown)
-- DoMeleeAttackIfReady()); with the interrupt arm unportable he effectively
-- never melees in C++. Melee is engine-driven in Go (creature combat tick)
-- and cannot be gated on the missing interrupt, so the port melees where
-- C++ does not — a genuine bridge gap.
-- Fight shape: every 1s a random normal bolt (frostbolt 29954 / fireball
-- 29953 / arc missile 29955 — the three 5s school cooldowns are only ever
-- armed by the interrupt arm (event 14 fires, but C++ needs the
-- SPELL_EFFECT_INTERRUPT_CAST effect data the hook does not carry, so the
-- arm has no Lua-modelable input and all three schools stay available);
-- every 5-20s a secondary (AOE counterspell
-- 29961 or chains of ice 29991); every 35-40s a super spell that never
-- repeats the previous one — arcane explosion (blink center 29967,
-- player pull 32265, mass slow 30035, then 29973), flame wreath (29946 on
-- up to 3 random players; a 500ms watcher punishes movement beyond 3 yd
-- from the cast point), or circular blizzard. Below 40% health once, four
-- water elementals; at 12 minutes and every 60s after, 5 shadow-of-Aran
-- summons (Talk + summon + Attack via EngageWithTarget). The drink/potion/pyroblast mana sequence needs
-- mana to drain; the repo carries no Spell.dbc, so creature spell costs
-- resolve to 0 and the mana pool never moves — see deviations.
-- Deviations from C++: the 4 water elementals (17167) ARE summoned now
-- (2026-10-22 summon bridge — creature:SummonCreature on the handler
-- surface; type 2 TIMED_DESPAWN, 90s) and their Attack(victim, true)
-- legs ARE ported via elemental:EngageWithTarget(victim) (2026-10-20
-- EngageWithTarget bridge; C++ order Attack-then-SetFaction kept; the
-- funnel is player-target-only, so a non-player victim no-ops). The 5
-- shadow-of-Aran summons (18254) ARE summoned now (2026-10-24 summon
-- bridge — type 3, 5s OOC grace) with the same Attack port; the blizzard
-- creature (17161) has no Attack leg in C++ (summon + SetFaction +
-- self-cast 29951 only). No C++ script exists for 18254/17161. The water elemental AI itself ("npc_aran_elemental") is
-- registered for entry 17167 below so the summoned elementals cast
-- waterbolt 31012 every 2-5s once engaged. No
-- instance-script model — the library-door open/close arms and the
-- DATA_ARAN SetBossState calls are skipped (admission-only via the
-- luaBossAI shim in engine/world/boss_ai.go). No threat model — the
-- flame-wreath target pool is random alive players in the instance rather
-- than the threat list. No UNIT_STATE_CASTING model — timers fire
-- unconditionally (same convention as the other Karazhan ports), and
-- InterruptNonMeleeSpells gates are skipped. SetPower (lua_creature_events.go:288), SetStandState (:568), and
-- GetPower/GetMaxPower (object.go:101) are all bridged — the old "no
-- GetPower" / "No SetPower/SetStandState" claims are stale. The whole
-- drink arm (SAY_DRINK=4, mass poly 29963, conjure 29975, drink 30024,
-- potion 32453, AOE pyroblast 29978, the drink-interrupt damage hook, the
-- 10s interrupt timer) is skipped anyway: creature casts deduct no mana
-- (no Spell.dbc, costs resolve to 0), so the C++ "< 20% mana" trigger can
-- never fire and a port would be dead code; SAY_4 never fires.
-- The drink aura's periodic mana refill has no tick executor in Go, so
-- the machine's refill model (drink ticks + SetPower(max-32000)) cannot be
-- reproduced — a partial port would re-drink in a loop. The SpellHit
-- interrupt arm stays out: CreatureEventOnHitBySpell
-- (14) is wired and fires with caster + spell id, but C++ gates on
-- spellInfo->Effects[i].Effect == SPELL_EFFECT_INTERRUPT_CAST and no
-- bridge exposes a spell's effect list. The flame-wreath 3-yd check is 2D
-- distance from the cast point — C++-exact
-- (unit->IsWithinDist2d(FWTargPosX[i], FWTargPosY[i], 3); no facing
-- involved); the C++ punishment is the moved unit casting 20476 on itself
-- with Aran as original caster — the bridge's CastSpell lives on the boss
-- object, so Aran casts 20476 onto the moved player instead (11027 via
-- player:AddAura, as C++ casts it triggered on the unit).

local ENTRY_SHADE_OF_ARAN = 16524
local GO_PRIVATE_LIBRARY_DOOR = 184517 -- karazhan.h: GO_PRIVATE_LIBRARY_DOOR
local ENTRY_WATER_ELEMENTAL = 17167
local ENTRY_ARAN_BLIZZARD = 17161
local ENTRY_SHADOW_OF_ARAN = 18254

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

-- C++ HealthBelowPct(pct) = GetHealth() < floor(maxHealth*pct/100)
-- (ironaya convention) — GetHealthPct() fires up to 1 HP early on the
-- float edge.
local function healthBelowPct(creature, pct)
    local max = creature:GetMaxHealth()
    if max == 0 then
        return false
    end
    return creature:GetHealth() < math.floor(max * pct / 100)
end

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
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ SelectTarget(Random, 0, dist, true): threat-list players alive
-- within range (UnitAI.h:162-176) — the old instance-player pool could
-- pick players who never engaged. GetAITargets is the bridged threat
-- list (lua_creature_events.go); the flame-wreath arm already uses it.
local function randomPlayer(creature, maxDist)
    local candidates = {}
    local reach = creature:GetCombatReach()
    for _, p in ipairs(creature:GetAITargets()) do
        if p:GetObjectType() == "Player" and not p:IsDead()
                and creature:GetDistance(p) - reach - 1.5 < maxDist then -- IsWithinCombatRange gate (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            candidates[#candidates + 1] = p
        end
    end
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
    -- C++ FlameWreathEffect (boss_shade_of_aran.cpp:184-203): up to 3
    -- random alive players from the threat list (random removal while > 3),
    -- no range cap. creature:GetAITargets() is the bridged threat list
    -- (lua_creature_events.go), replacing the retired alivePlayers
    -- instance approx 2026-10-23.
    local pool = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            pool[#pool + 1] = unit
        end
    end
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
                -- C++ unit->CastSpell(unit, 20476, me->GetGUID()) resolves to
                -- CastSpellExtraArgs(ObjectGuid) -> TRIGGERED_FULL_MASK
                -- (SpellDefines.h: the third-arg GUID ctor is a triggered
                -- cast), so the explosion is fully triggered — instant,
                -- ignores GCD/cast-in-progress/power costs; the bridge's
                -- third arg is the Eluna triggered flag.
                creature:CastSpell(unit, SPELL_FLAMEWREATH_PUNISH, true)
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
        if not st.elementalsSpawned and healthBelowPct(creature, 40) then
            st.elementalsSpawned = true
            -- C++ boss_shade_of_aran.cpp:419-431: 4x SummonCreature(
            -- CREATURE_WATER_ELEMENTAL = 17167, 0,0,0,0,
            -- TEMPSUMMON_TIMED_DESPAWN (2), 90s = 90000ms). The all-zero
            -- coords ride the bridge's Object.cpp:2059-2060 close-point
            -- fallback (C++-exact). unit->Attack(me->GetVictim(), true)
            -- ports via elemental:EngageWithTarget(victim) in C++ order
            -- (Attack, then SetFaction); the funnel is player-target-only.
            local victim = creature:GetVictim()
            for _ = 1, 4 do
                local elemental = creature:SummonCreature(ENTRY_WATER_ELEMENTAL,
                    0, 0, 0, 0, 2, 90000)
                if elemental ~= nil then
                    if victim ~= nil then
                        elemental:EngageWithTarget(victim)
                    end
                    elemental:SetFaction(creature:GetFaction())
                end
            end
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
        -- C++ boss_shade_of_aran.cpp:402-406: SummonCreature(
        -- CREATURE_ARAN_BLIZZARD = 17161, 0,0,0,0,
        -- TEMPSUMMON_TIMED_DESPAWN (2), 25s = 25000ms); the all-zero
        -- coords ride the bridge's Object.cpp:2059-2060 close-point
        -- fallback (elemental precedent). No C++ script exists for
        -- 17161 — its sole act is the summon-time self-cast, ported
        -- here; SetFaction is bridged and ported C++-exact.
        local blizzard = creature:SummonCreature(ENTRY_ARAN_BLIZZARD,
            0, 0, 0, 0, 2, 25000)
        if blizzard ~= nil then
            blizzard:SetFaction(creature:GetFaction())
            blizzard:CastSpell(blizzard, SPELL_CIRCULAR_BLIZZARD)
        end
    end
    schedule(timers, guid, "super", math.random(35000, 40000), function()
        onSuperCast(creature, guid)
    end)
end

onBerserk = function(creature, guid)
    -- C++ boss_shade_of_aran.cpp:433-443: 5x SummonCreature(
    -- CREATURE_SHADOW_OF_ARAN = 18254, 0,0,0,0,
    -- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT (3), 5s = 5000ms); the
    -- all-zero coords ride the bridge's close-point fallback and the
    -- type-3 timer freezes in combat (pet_combat.go expiry twin),
    -- so the 5s runs only out of combat, C++-exact. No C++ script
    -- exists for 18254 — unit->Attack(victim) ports via
    -- shadow:EngageWithTarget(victim) in C++ order (Attack, then
    -- SetFaction); the funnel is player-target-only.
    local victim = creature:GetVictim()
    for _ = 1, 5 do
        local shadow = creature:SummonCreature(ENTRY_SHADOW_OF_ARAN,
            0, 0, 0, 0, 3, 5000)
        if shadow ~= nil then
            if victim ~= nil then
                shadow:EngageWithTarget(victim)
            end
            shadow:SetFaction(creature:GetFaction())
        end
    end
    creature:Talk(SAY_TIMEOVER)
    schedule(timers, guid, "berserk", 60000, function()
        onBerserk(creature, guid)
    end)
end

local function aranEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    aranInitState(guid)
    -- C++ JustEngagedWith: Talk(SAY_AGGRO) + library-door close
    -- (HandleGameObject(DATA_GO_LIBRARY_DOOR, false) = GO_STATE_READY;
    -- the 15s CloseDoorTimer close in UpdateAI is a redundant no-op once
    -- the engage close lands). SetBossState(IN_PROGRESS) has no bridge.
    creature:Talk(SAY_AGGRO)
    local libraryDoor = creature:FindNearestGameObject(GO_PRIVATE_LIBRARY_DOOR, 100.0)
    if libraryDoor then libraryDoor:SetGoState(1) end
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
    -- C++ JustDied: Talk(SAY_DEATH) + library-door re-open
    -- (HandleGameObject true = GO_STATE_ACTIVE); SetBossState(DONE) has
    -- no bridge.
    creature:Talk(SAY_DEATH)
    local libraryDoor = creature:FindNearestGameObject(GO_PRIVATE_LIBRARY_DOOR, 100.0)
    if libraryDoor then libraryDoor:SetGoState(0) end
end

local function aranReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    aranInitState(guid)
    -- C++ Reset: library-door open (HandleGameObject true = GO_STATE_ACTIVE).
    local libraryDoor = creature:FindNearestGameObject(GO_PRIVATE_LIBRARY_DOOR, 100.0)
    if libraryDoor then libraryDoor:SetGoState(0) end
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
    creature:SetMeleeAttack(false) -- boss_shade_of_aran.cpp:536: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
    local guid = creature:GetGUID()
    cancelTimers(elementalTimers, guid)
    -- C++ water_elementalAI Reset: CastTimer = 2000 + rand32() % 3000
    -- ([2000,4999], exclusive top); combat re-arms at urand(2000, 5000).
    schedule(elementalTimers, guid, "waterbolt", math.random(2000, 4999), function()
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
