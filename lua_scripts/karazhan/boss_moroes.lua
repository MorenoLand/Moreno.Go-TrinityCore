-- Moroes (Karazhan) — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/boss_moroes.cpp
-- Creature entries: 15687 (Moroes, TDB creature_template ScriptName
-- "boss_moroes"); guests 17007 (baroness dorothea, shadow priest),
-- 19872 (baron rafe, ret paladin), 19873 (lady catriona, holy priest),
-- 19874 (lady keira, holy paladin), 19875 (lord robin, arms warrior),
-- 19876 (lord crispin, arms warrior).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent; melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature casts
-- are packet-visual), so timers fire unconditionally; the 5s no-melee
-- vanish window is not modeled (engine melee is continuous); no Karazhan
-- instance-script model — Reset/aggro/death do not SetBossState and the
-- guest "evade while the encounter is not IN_PROGRESS" arm is skipped
-- (encounter admission still binds through the luaBossAI shim in
-- engine/world/boss_ai.go); Reset's SpawnAdds / JustDied's DeSpawnAdds
-- are ported on the live SummonCreature bridge
-- (lua_creature_events.go:877; Lua type = C++ TempSummonType - 1, the
-- Object.cpp:2109 twin): the first Reset picks 4 random distinct guests
-- of the 6 (C++ RandomResize), later resets respawn the same four at
-- Locations[i] (CORPSE_TIMED_DESPAWN, 10s) after a by-entry despawn
-- sweep (no GUID->creature lookup — the ossirian onReset precedent);
-- the six guests' class AIs are registered in this file so the summons
-- fight. The healer guests' AcquireGUID/
-- SelectGuestTarget need instance GetGuidData, so they target themselves;
-- JustDied's DoRemoveAurasDueToSpellOnPlayers(GARROTE) is ported (arm
-- order after Talk(SAY_DEATH), boss_moroes.cpp:181) via GetPlayersInWorld
-- + mapId/instanceId filter and player:RemoveAura (bridged).
-- JustEngagedWith's AddsAttack and UpdateAI's 5s CheckAdds re-aggro arm
-- (AttackStart on the boss's victim for adds without one) have no
-- AttackStart/DoZoneInCombat/EnterEvadeMode bridges (the keleseth
-- guard-AttackStart precedent) — the adds engage via the engine's own
-- aggro.

local ENTRY_MOROES = 15687
local ENTRY_DOROTHEA, ENTRY_RAFE, ENTRY_CATRIONA = 17007, 19872, 19873
local ENTRY_KEIRA, ENTRY_ROBIN, ENTRY_CRISPIN = 19874, 19875, 19876

-- C++ boss_moroes.cpp:86-94 (Adds[6]) and :78-84 (Locations[4]).
local MOROES_GUEST_POOL = {17007, 19872, 19873, 19874, 19875, 19876}
local MOROES_GUEST_LOCS = {
    {-10991.0, -1884.33, 81.73, 0.614315},
    {-10989.4, -1885.88, 81.73, 0.904913},
    {-10978.1, -1887.07, 81.73, 2.035550},
    {-10975.9, -1885.81, 81.73, 2.253890},
}

local SAY_AGGRO, SAY_SPECIAL, SAY_KILL, SAY_DEATH = 0, 1, 2, 3

local SPELL_VANISH = 29448
local SPELL_GARROTE = 37066
local SPELL_BLIND = 34694
local SPELL_GOUGE = 29425
local SPELL_FRENZY = 37023

local SPELL_MANABURN = 29405
local SPELL_MINDFLY = 29570
local SPELL_SWPAIN = 34441
local SPELL_SHADOWFORM = 29406
local SPELL_HAMMEROFJUSTICE = 13005
local SPELL_JUDGEMENTOFCOMMAND = 29386
local SPELL_SEALOFCOMMAND = 29385
local SPELL_DISPELMAGIC = 15090
local SPELL_GREATERHEAL = 29564
local SPELL_HOLYFIRE = 29563
local SPELL_PWSHIELD = 29408
local SPELL_CLEANSE = 29380
local SPELL_GREATERBLESSOFMIGHT = 29381
local SPELL_HOLYLIGHT = 29562
local SPELL_DIVINESHIELD = 41367
local SPELL_HAMSTRING = 9080
local SPELL_MORTALSTRIKE = 29572
local SPELL_WHIRLWIND = 29573
local SPELL_DISARM = 8379
local SPELL_HEROICSTRIKE = 29567
local SPELL_SHIELDBASH = 11972
local SPELL_SHIELDWALL = 29390

local POWER_MANA = 0

local timers = {}

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

local function randomTargetInRange(creature, range)
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if creature:IsWithinDist(p, range) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- Blind: SelectTarget(MinDistance, 0, 0.0f, true, false) — the 5th arg
-- withTank=false (UnitAI.h:170) excludes the tank; approximated as the
-- nearest alive player that is not the current victim.
local function nearestTarget(creature)
    local victim = creature:GetVictim()
    local victimGuid = victim ~= nil and victim:GetGUID() or nil
    local best, bestDist = nil, nil
    for _, p in ipairs(playersInInstance(creature)) do
        if p:GetGUID() ~= victimGuid then
            local dist = creature:GetDistance(p)
            if bestDist == nil or dist < bestDist then
                best, bestDist = p, dist
            end
        end
    end
    return best
end

-- Moroes ---------------------------------------------------------------

local moroesState = {}

local function moroesInitState(guid)
    -- C++ boss_moroesAI ctor memsets AddId to 0; AddId persists on the AI
    -- across Reset/engage/death (only the ctor clears it), so the 4
    -- picked guests survive state re-inits.
    local prev = moroesState[guid]
    moroesState[guid] = {
        enrage = false,
        invanish = false,
        addIds = prev ~= nil and prev.addIds or nil,
    }
end

-- C++ boss_moroes.cpp:213-222 DeSpawnAdds — DespawnOrUnsummon each add by
-- GUID. Lua has no GUID->creature lookup, so the adds are despawned by
-- entry sweep in grid range (the ossirian onReset precedent).
local function despawnAdds(creature)
    for _, entry in ipairs(MOROES_GUEST_POOL) do
        for _, add in ipairs(creature:GetCreaturesInGrid(entry, 60)) do
            add:Despawn()
        end
    end
end

local function onGarrote(creature, guid)
    local state = moroesState[guid]
    if state == nil then
        return
    end
    creature:Talk(SAY_SPECIAL)
    local target = randomTargetInRange(creature, 100)
    if target then
        -- C++ boss_moroes.cpp:315: target->CastSpell(target, SPELL_GARROTE,
        -- true) — but player:CastSpell has no bridge (CastSpell is
        -- registered for creature objects only,
        -- engine/world/lua_creature_events.go:60); the target-self-cast
        -- would nil-call and kill this timer chain, so the creature
        -- casts on the target instead (caster deviates from C++).
        creature:CastSpell(target, SPELL_GARROTE, true)
    end
    state.invanish = false
end

local function onVanish(creature, guid)
    local state = moroesState[guid]
    if state == nil or state.enrage then
        -- C++ UpdateAI gates vanish/blind/gouge on !Enrage: once frenzy
        -- fires at <30% no new vanish casts land (onBlind/onGouge already
        -- gate this way).
        return
    end
    creature:CastSpell(creature, SPELL_VANISH)
    state.invanish = true
    schedule(guid, "vanish", 30000, function() onVanish(creature, guid) end)
    schedule(guid, "garrote", 5000, function() onGarrote(creature, guid) end)
end

local function onBlind(creature, guid)
    local state = moroesState[guid]
    if state == nil or state.enrage then
        return
    end
    local target = nearestTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_BLIND)
    end
    schedule(guid, "blind", 40000, function() onBlind(creature, guid) end)
end

local function onGouge(creature, guid)
    local state = moroesState[guid]
    if state == nil or state.enrage then
        return
    end
    creature:CastSpell(nil, SPELL_GOUGE)
    schedule(guid, "gouge", 40000, function() onGouge(creature, guid) end)
end

local function moroesEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    moroesInitState(guid)
    -- C++ boss_moroes.cpp:155+164: JustEngagedWith calls StartEvent()
    -- (SetBossState IN_PROGRESS + DoZoneInCombat()) then Talk(SAY_AGGRO),
    -- AddsAttack(), DoZoneInCombat() — one ported call in arm order (the
    -- AddsAttack cross-creature leg stays out, documented below).
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    schedule(guid, "vanish", 30000, function() onVanish(creature, guid) end)
    schedule(guid, "blind", 35000, function() onBlind(creature, guid) end)
    schedule(guid, "gouge", 23000, function() onGouge(creature, guid) end)
    -- C++ JustEngagedWith's AddsAttack (boss_moroes.cpp:224-238) —
    -- temp->AI()->AttackStart(me->GetVictim()) + DoZoneInCombat(temp)
    -- for each live add, EnterEvadeMode() if any add is missing — has
    -- no bearer: no AttackStart / DoZoneInCombat / EnterEvadeMode
    -- bridges (the keleseth guard-AttackStart precedent). The four
    -- summoned guests run their registered class AIs and engage via
    -- the engine's own aggro.
end

-- Enrage: C++ UpdateAI checks HealthBelowPct(30) on the post-damage health
-- (Unit.h:872) every tick; the engine fires event 9 pre-damage
-- (lua_creature_events.go:424-425), so `health` is not the C++ value.
-- HealthBelowPctDamaged(30, damage) projection matches (malchezaar
-- precedent); integer floor form matches CalculatePct exactly.
-- C++ is UpdateAI post-damage, so it never fires on the killing blow;
-- the postDamage > 0 guard (nefarian rule) keeps the port from frenzying
-- while dying.
local function moroesDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local state = moroesState[guid]
    if state == nil or state.enrage then
        return
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local postDamage = math.max(health - damage, 0)
    if postDamage > 0 and postDamage < math.floor(maxHealth * 30 / 100) then
        creature:CastSpell(creature, SPELL_FRENZY)
        state.enrage = true
    end
end

local function moroesLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function moroesReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    moroesInitState(guid)
    -- Go fires event 23 on evade and on the death path (before event 4);
    -- the death path is guarded like the overgrind precedent (C++ Reset
    -- only runs SpawnAdds when the boss is alive, boss_moroes.cpp:146).
    if creature:IsDead() then
        return
    end
    -- C++ Reset (boss_moroes.cpp:143-149): Initialize(); if
    -- (me->IsAlive()) SpawnAdds(). SpawnAdds (:184-211) first
    -- DeSpawnAdds, then on the first Reset picks 4 random distinct
    -- guests of the 6 (RandomResize) and summons each at Locations[i]
    -- (TEMPSUMMON_CORPSE_TIMED_DESPAWN, 10s); later resets respawn the
    -- same four by AddId. Lua type 5 = C++ type - 1 (Object.cpp:2109
    -- twin, the 12:04 summonleg precedent).
    despawnAdds(creature)
    local state = moroesState[guid]
    if state.addIds == nil then
        local pool = {17007, 19872, 19873, 19874, 19875, 19876}
        local picked = {}
        for i = 1, 4 do
            local idx = math.random(#pool)
            picked[i] = pool[idx]
            table.remove(pool, idx)
        end
        state.addIds = picked
    end
    for i = 1, 4 do
        local loc = MOROES_GUEST_LOCS[i]
        creature:SummonCreature(state.addIds[i], loc[1], loc[2], loc[3], loc[4], 5, 10000)
    end
end

local function moroesTargetDied(event, creature, victim)
    creature:Talk(SAY_KILL)
end

local function moroesDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    moroesInitState(guid)
    creature:Talk(SAY_DEATH)
    -- C++-exact arm order: instance->SetBossState(DONE) rides the
    -- no-instance bridge; DeSpawnAdds (boss_moroes.cpp:175) ports here,
    -- then DoRemoveAurasDueToSpellOnPlayers(SPELL_GARROTE) (:181).
    despawnAdds(creature)
    -- C++-exact arm order: instance->DoRemoveAurasDueToSpellOnPlayers(
    -- SPELL_GARROTE) (boss_moroes.cpp:181) — the SetBossState leg rides
    -- the no-instance bridge. No death filter: C++
    -- iterates every instance player (mapId/instanceId match, maiden
    -- precedent); player:RemoveAura is bridged (lua_player.go:219).
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId then
            p:RemoveAura(SPELL_GARROTE)
        end
    end
end

RegisterCreatureEvent(ENTRY_MOROES, 1, moroesEnterCombat)
RegisterCreatureEvent(ENTRY_MOROES, 2, moroesLeaveCombat)
RegisterCreatureEvent(ENTRY_MOROES, 3, moroesTargetDied)
RegisterCreatureEvent(ENTRY_MOROES, 4, moroesDied)
RegisterCreatureEvent(ENTRY_MOROES, 9, moroesDamageTaken)
RegisterCreatureEvent(ENTRY_MOROES, 23, moroesReset)

-- Guests ----------------------------------------------------------------

-- Healer guests (catriona, keira) pick a random alive co-add via the
-- instance; with no instance-script model the fallback is the caster
-- itself, like C++'s SelectGuestTarget returning me when no add is found.
local function guestHealTarget(creature)
    return creature
end

local function guestReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function guestEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
end

-- Dorothea (shadow priest): MindFlay on the victim, ManaBurn picking one
-- random player within 100 and casting only if that target is a mana user
-- (C++ boss_moroes.cpp:424-430: SelectTarget(Random, 0, 100, true), tank
-- eligible, cast gated on target power type — probability M/N per 5s, not
-- guaranteed), ShadowWordPain on a random player within 100
-- (the repeat only re-arms when a target was found, like C++).
local function dorotheaMindFlay(creature, guid)
    creature:CastSpell(nil, SPELL_MINDFLY)
    schedule(guid, "mindflay", 12000, function() dorotheaMindFlay(creature, guid) end)
end

local function dorotheaManaBurn(creature, guid)
    local target = randomTargetInRange(creature, 100)
    if target and target:GetPowerType() == POWER_MANA then
        creature:CastSpell(target, SPELL_MANABURN)
    end
    schedule(guid, "manaburn", 5000, function() dorotheaManaBurn(creature, guid) end)
end

local function dorotheaSwPain(creature, guid)
    local target = randomTargetInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_SWPAIN)
        schedule(guid, "swpain", 7000, function() dorotheaSwPain(creature, guid) end)
    else
        -- C++ leaves the expired timer armed, retrying next tick; a 1s
        -- poll is the CreateLuaEvent analogue.
        schedule(guid, "swpain", 1000, function() dorotheaSwPain(creature, guid) end)
    end
end

local function dorotheaEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    guestEnterCombat(event, creature, target)
    schedule(guid, "mindflay", 1000, function() dorotheaMindFlay(creature, guid) end)
    schedule(guid, "manaburn", 7000, function() dorotheaManaBurn(creature, guid) end)
    schedule(guid, "swpain", 6000, function() dorotheaSwPain(creature, guid) end)
end

local function dorotheaReset(event, creature)
    guestReset(event, creature)
    -- C++ Reset(): DoCast(me, SHADOWFORM, true) unconditionally
    -- (boss_moroes.cpp).
    creature:CastSpell(creature, SPELL_SHADOWFORM, true)
end

RegisterCreatureEvent(ENTRY_DOROTHEA, 1, dorotheaEnterCombat)
RegisterCreatureEvent(ENTRY_DOROTHEA, 2, guestReset)
RegisterCreatureEvent(ENTRY_DOROTHEA, 4, guestReset)
RegisterCreatureEvent(ENTRY_DOROTHEA, 23, dorotheaReset)

-- Rafe (ret paladin): Seal arms Judgement 29s after each cast — the seal
-- fire replaces the judgement key and the judgement self-repeat of 32s
-- lands on the same beat (seal period 32 + 29 = 61, judgement at seal+29
-- repeating on 32), matching C++'s SealOfCommand_Timer+29000 assignment.
local function rafeHammer(creature, guid)
    creature:CastSpell(nil, SPELL_HAMMEROFJUSTICE)
    schedule(guid, "hammer", 12000, function() rafeHammer(creature, guid) end)
end

local function rafeJudgement(creature, guid)
    creature:CastSpell(nil, SPELL_JUDGEMENTOFCOMMAND)
    schedule(guid, "judgement", 32000, function() rafeJudgement(creature, guid) end)
end

local function rafeSeal(creature, guid)
    creature:CastSpell(creature, SPELL_SEALOFCOMMAND)
    schedule(guid, "seal", 32000, function() rafeSeal(creature, guid) end)
    schedule(guid, "judgement", 29000, function() rafeJudgement(creature, guid) end)
end

local function rafeEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    guestEnterCombat(event, creature, target)
    schedule(guid, "hammer", 1000, function() rafeHammer(creature, guid) end)
    schedule(guid, "seal", 7000, function() rafeSeal(creature, guid) end)
    schedule(guid, "judgement", 36000, function() rafeJudgement(creature, guid) end)
end

RegisterCreatureEvent(ENTRY_RAFE, 1, rafeEnterCombat)
RegisterCreatureEvent(ENTRY_RAFE, 2, guestReset)
RegisterCreatureEvent(ENTRY_RAFE, 4, guestReset)
RegisterCreatureEvent(ENTRY_RAFE, 23, guestReset)

-- Catriona (holy priest): heals/shields the guest target (self, see above),
-- HolyFire on the victim, DispelMagic on either target 50/50.
local function catrionaPwShield(creature, guid)
    creature:CastSpell(creature, SPELL_PWSHIELD)
    schedule(guid, "pwshield", 15000, function() catrionaPwShield(creature, guid) end)
end

local function catrionaGreaterHeal(creature, guid)
    creature:CastSpell(guestHealTarget(creature), SPELL_GREATERHEAL)
    schedule(guid, "greaterheal", 17000, function() catrionaGreaterHeal(creature, guid) end)
end

local function catrionaHolyFire(creature, guid)
    creature:CastSpell(nil, SPELL_HOLYFIRE)
    schedule(guid, "holyfire", 22000, function() catrionaHolyFire(creature, guid) end)
end

local function catrionaDispel(creature, guid)
    local target = guestHealTarget(creature)
    if math.random() < 0.5 then
        target = randomTargetInRange(creature, 100)
    end
    if target then
        creature:CastSpell(target, SPELL_DISPELMAGIC)
    end
    schedule(guid, "dispel", 25000, function() catrionaDispel(creature, guid) end)
end

local function catrionaEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    guestEnterCombat(event, creature, target)
    schedule(guid, "pwshield", 1000, function() catrionaPwShield(creature, guid) end)
    schedule(guid, "greaterheal", 1500, function() catrionaGreaterHeal(creature, guid) end)
    schedule(guid, "holyfire", 5000, function() catrionaHolyFire(creature, guid) end)
    schedule(guid, "dispel", 11000, function() catrionaDispel(creature, guid) end)
end

RegisterCreatureEvent(ENTRY_CATRIONA, 1, catrionaEnterCombat)
RegisterCreatureEvent(ENTRY_CATRIONA, 2, guestReset)
RegisterCreatureEvent(ENTRY_CATRIONA, 4, guestReset)
RegisterCreatureEvent(ENTRY_CATRIONA, 23, guestReset)

-- Keira (holy paladin).
local function keiraCleanse(creature, guid)
    creature:CastSpell(guestHealTarget(creature), SPELL_CLEANSE)
    schedule(guid, "cleanse", 10000, function() keiraCleanse(creature, guid) end)
end

local function keiraBless(creature, guid)
    creature:CastSpell(guestHealTarget(creature), SPELL_GREATERBLESSOFMIGHT)
    schedule(guid, "bless", 50000, function() keiraBless(creature, guid) end)
end

local function keiraHolyLight(creature, guid)
    creature:CastSpell(guestHealTarget(creature), SPELL_HOLYLIGHT)
    schedule(guid, "holylight", 10000, function() keiraHolyLight(creature, guid) end)
end

local function keiraDivineShield(creature, guid)
    creature:CastSpell(creature, SPELL_DIVINESHIELD)
    schedule(guid, "divineshield", 31000, function() keiraDivineShield(creature, guid) end)
end

local function keiraEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    guestEnterCombat(event, creature, target)
    schedule(guid, "cleanse", 13000, function() keiraCleanse(creature, guid) end)
    schedule(guid, "bless", 1000, function() keiraBless(creature, guid) end)
    schedule(guid, "holylight", 7000, function() keiraHolyLight(creature, guid) end)
    schedule(guid, "divineshield", 31000, function() keiraDivineShield(creature, guid) end)
end

RegisterCreatureEvent(ENTRY_KEIRA, 1, keiraEnterCombat)
RegisterCreatureEvent(ENTRY_KEIRA, 2, guestReset)
RegisterCreatureEvent(ENTRY_KEIRA, 4, guestReset)
RegisterCreatureEvent(ENTRY_KEIRA, 23, guestReset)

-- Robin (arms warrior).
local function robinHamstring(creature, guid)
    creature:CastSpell(nil, SPELL_HAMSTRING)
    schedule(guid, "hamstring", 12000, function() robinHamstring(creature, guid) end)
end

local function robinMortalStrike(creature, guid)
    creature:CastSpell(nil, SPELL_MORTALSTRIKE)
    schedule(guid, "mortalstrike", 18000, function() robinMortalStrike(creature, guid) end)
end

local function robinWhirlwind(creature, guid)
    creature:CastSpell(creature, SPELL_WHIRLWIND)
    schedule(guid, "whirlwind", 21000, function() robinWhirlwind(creature, guid) end)
end

local function robinEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    guestEnterCombat(event, creature, target)
    schedule(guid, "hamstring", 7000, function() robinHamstring(creature, guid) end)
    schedule(guid, "mortalstrike", 10000, function() robinMortalStrike(creature, guid) end)
    schedule(guid, "whirlwind", 21000, function() robinWhirlwind(creature, guid) end)
end

RegisterCreatureEvent(ENTRY_ROBIN, 1, robinEnterCombat)
RegisterCreatureEvent(ENTRY_ROBIN, 2, guestReset)
RegisterCreatureEvent(ENTRY_ROBIN, 4, guestReset)
RegisterCreatureEvent(ENTRY_ROBIN, 23, guestReset)

-- Crispin (arms warrior).
local function crispinDisarm(creature, guid)
    creature:CastSpell(nil, SPELL_DISARM)
    schedule(guid, "disarm", 12000, function() crispinDisarm(creature, guid) end)
end

local function crispinHeroicStrike(creature, guid)
    creature:CastSpell(nil, SPELL_HEROICSTRIKE)
    schedule(guid, "heroicstrike", 10000, function() crispinHeroicStrike(creature, guid) end)
end

local function crispinShieldBash(creature, guid)
    creature:CastSpell(nil, SPELL_SHIELDBASH)
    schedule(guid, "shieldbash", 13000, function() crispinShieldBash(creature, guid) end)
end

local function crispinShieldWall(creature, guid)
    creature:CastSpell(creature, SPELL_SHIELDWALL)
    schedule(guid, "shieldwall", 21000, function() crispinShieldWall(creature, guid) end)
end

local function crispinEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    guestEnterCombat(event, creature, target)
    schedule(guid, "disarm", 6000, function() crispinDisarm(creature, guid) end)
    schedule(guid, "heroicstrike", 10000, function() crispinHeroicStrike(creature, guid) end)
    schedule(guid, "shieldbash", 8000, function() crispinShieldBash(creature, guid) end)
    schedule(guid, "shieldwall", 4000, function() crispinShieldWall(creature, guid) end)
end

RegisterCreatureEvent(ENTRY_CRISPIN, 1, crispinEnterCombat)
RegisterCreatureEvent(ENTRY_CRISPIN, 2, guestReset)
RegisterCreatureEvent(ENTRY_CRISPIN, 4, guestReset)
RegisterCreatureEvent(ENTRY_CRISPIN, 23, guestReset)
