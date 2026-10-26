-- Amanitar (Ahn'kahet) — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/Ahnkahet/boss_amanitar.cpp
-- (boss_amanitar + npc_amanitar_mushrooms +
-- spell_amanitar_potent_fungus).
-- Ahn'kahet dungeon-script unit per northrend_script_loader.cpp order
-- (decl 34 / call 224, immediately after AddSC_boss_taldaram();
-- next: AddSC_boss_jedoga_shadowseeker() (decl 35 / call 225), then
-- AddSC_boss_volazj() (decl 36 / call 226, script boss_herald_volazj)).
-- Entry: 30258 Amanitar (ahnkahet.h NPC_AMANITAR — kalecgos pass; the
-- RegisterAhnKahetCreatureAI ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as
-- usual).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim, creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell[, triggered]) = DoCastSelf
-- (phase_hunter / apothecary_hanes precedent — the trailing true is
-- the triggered port).
-- Ported arms (C++-exact for all modeled arms):
-- boss_amanitar: EVENT_BASH — DoCastVictim(Bash 57094);
-- JustEngagedWith-scheduled 10-14s init, rescheduled 7-12s;
-- EVENT_MINI — SelectTarget(Random, 0, 0.0f, true, true,
-- -SPELL_MINI 57055): random alive player WITHOUT the Mini aura
-- (the negative-aura filter is a player:HasAura(57055) exclusion
-- in the GetAITargets pool — netherspite/vaelastrasz precedent;
-- the old "random-target SelectTarget bridge absent" header claim
-- was stale, retracted 2026-10-18) -> DoCastAOE(Mini 57055)
-- self-cast non-triggered (arugal DoCastAOE convention); 12-18s
-- init, 30s repeat on cast, 1s fail-fast repeat when no valid
-- target (C++ :141-147 — the else branch re-arms 1s, not 30s);
-- EVENT_ROOT — SelectTarget(Random, 1, 100.0f, true):
-- offset-1 skip of the first MaxThreat entry BEFORE player/dist
-- filters (UnitAI.h; eck precedent — 2026-10-25 fix, old
-- victim-GUID exclusion was wrong when top threat != victim),
-- alive threat target within 100yd, playerOnly=true (the 4th arg,
-- murmur convention) -> DoCast(target, Entangling Roots 57095) TRIGGERED
-- (trailing true — apothecary_hanes convention); 5-9s init,
-- 10-15s repeat (the re-arm sits outside the target check —
-- C++ :150-152 exact); EVENT_BOLT — SelectTarget(Random, 0,
-- 100.0f, true): playerOnly=true (the 4th arg), random alive
-- player within 100yd (position 0 = no offset) -> DoCast(target,
-- Venom Bolt Volley 57088) TRIGGERED; 15-20s init, 18-22s
-- repeat (C++ :156-160).
-- JustDied — DoCastAOE(Remove Mushroom Power 57283), self-cast
-- (arugal precedent).
-- Unmodeled (no bridges — documented, not wired):
-- EVENT_SPAWN is PORTED (spawnTick — the stale "summon STRAND
-- absent" claim is retracted for this one-shot summon leg).
-- EVENT_RESPAWN is PORTED (respawnTick — ev21 deque + 40-60s repeat,
-- amanitarSummonedDied below).
-- JustSummoned summons.Summon leg: moot in Lua (no summon-list
-- container; the spawnTick SummonCreature calls arm the timer via
-- the Eluna spawn type directly).
-- EnterEvadeMode — summons.DespawnAll (summon-lifecycle STRAND
-- absent — the old "no despawn bridge (terestian precedent)" was
-- stale: creature:Despawn() IS bridged (lua_creature_events.go:236),
-- but the arm needs the whole summon list), the
-- DoRemoveAurasDueToSpellOnPlayers(Mini) player leg (SPELL_MINI 57055)
-- is PORTED via stripMiniAuras (the includePets/includeControlled legs
-- have no pet/controlled iteration bridge — stay out),
-- _DespawnAtEvade.
-- JustDied — _JustDied() instance bookkeeping only (no bridge);
-- DoCastAOE(Remove Mushroom Power 57283) ported as self-cast
-- (creature:CastSpell(creature, 57283) — the Go creature-cast path
-- carries no alive-gate on the caster, so the OnDied cast is
-- faithful); the instance DoRemoveAurasDueToSpellOnPlayers(Mini) player
-- leg (SPELL_MINI 57055) is PORTED via stripMiniAuras (the
-- includePets/includeControlled legs have no pet/controlled iteration
-- bridge — stay out).
-- npc_amanitar_mushrooms (30391 NPC_HEALTHY_MUSHROOM / 30435
-- NPC_POISONOUS_MUSHROOM — ahnkahet.h kalecgos pass) — PORTED
-- 2026-10-24 via the summoner-ev19 vehicle: Reset leg —
-- SetReactState(REACT_PASSIVE) + SetDisplayId(Modelid2 via
-- creature:GetTemplateModelId(2) — the old "unbridged Modelid2 VALUE
-- bearer" claim was stale) + DoCastSelf triplet (Putrid Mushroom
-- 31690 / Shrink 31691 triggered / Grow 57059 triggered) +
-- healthy/poisonous visual-aura self-cast (56740 / 56741) + the
-- 800ms re-Grow one-shot (amanitarJustSummoned); JustDied healthy
-- leg — DoCastAOE(Potent Fungus 56648, triggered) — entry-registered
-- (mushroomDied, event 4). The MoveInLineOfSight player-proximity
-- poison leg (2.0f, _active latch) stays out: ev27 fires only from
-- the aggro-engage branch and passive mushrooms never engage;
-- target RemoveAurasDueToSpell(Potent Fungus 56648) (player:RemoveAura
-- IS bridged), DoCastAOE(Poisonous Mushroom Poison Cloud 57061),
-- SetObjectScale(0.1f) (scale bridge exists, creature:SetScale),
-- DespawnOrUnsummon(4s) (creature:Despawn() bridged) all ride the
-- unported proximity trigger out.
-- spell_amanitar_potent_fungus — AuraScript (56648 Potent Fungus:
-- on-apply, if the target HasAura(Mini 57055) remove it and drop
-- the aura): no SpellScript/AuraScript binding bridge (razelikh
-- precedent) — documented-only.
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler
-- (gargolmar precedent); BossAI _Reset/_JustDied instance
-- bookkeeping has no bridge.

local ENTRY_AMANITAR = 30258

local SPELL_BASH = 57094
local SPELL_MINI = 57055
local SPELL_ENTANGLING_ROOTS = 57095
local SPELL_VENOM_BOLT_VOLLEY = 57088
local SPELL_REMOVE_MUSHROOM_POWER = 57283
local SPELL_POTENT_FUNGUS = 56648
local SPELL_POISONOUS_MUSHROOM_VISUAL_AURA = 56741
local SPELL_POWER_MUSHROOM_VISUAL_AURA = 56740
local SPELL_PUTRID_MUSHROOM = 31690
local SPELL_GROW = 57059
local SPELL_SHRINK = 31691

-- boss_amanitar.cpp :52-82 (ahnkahet.h NPC_HEALTHY_MUSHROOM 30391,
-- NPC_POISONOUS_MUSHROOM 30435).
local NPC_HEALTHY_MUSHROOM = 30391
local NPC_POISONOUS_MUSHROOM = 30435
local MUSHROOM_POSITIONS = {
    {373.4807, -856.5301, -74.30518, 0.2094395},
    {358.4792, -879.3193, -75.9463, 5.166174},
    {356.5531, -846.3022, -72.1796, 3.193953},
    {332.369, -846.081, -74.30516, 4.834562},
    {360.2234, -862.055, -75.22755, 1.658063},
    {351.7189, -890.9619, -76.54617, 1.064651},
    {345.8126, -869.1772, -77.17728, 1.361357},
    {367.5179, -884.0129, -77.32881, 4.276057},
    {370.6044, -868.4305, -74.19881, 0.8901179},
    {381.3156, -873.2377, -74.82656, 1.099557},
    {371.5869, -873.8141, -74.72424, 1.082104},
    {340.4079, -891.6375, -74.99128, 1.134464},
    {368.21, -851.5953, -73.99741, 4.694936},
    {328.7047, -853.9812, -75.51253, 0.5759587},
    {366.4145, -876.39, -75.52739, 5.253441},
    {380.1362, -861.4344, -73.45917, 3.787364},
    {373.3007, -888.8057, -79.03593, 5.602507},
    {348.3599, -848.0839, -73.54117, 1.745329},
    {352.5586, -882.6624, -75.68202, 3.822271},
    {357.8967, -871.179, -75.77553, 2.443461},
    {360.1034, -842.3351, -71.08852, 4.34587},
    {348.1334, -861.5244, -74.61307, 2.565634},
    {401.4896, -866.7059, -73.22395, 0.8901179},
    {360.1683, -889.1515, -76.74798, 3.612832},
    {350.1828, -907.7313, -74.94678, 5.044002},
    {340.6278, -856.5973, -74.23862, 4.415683},
    {366.4849, -859.7621, -74.82679, 1.500983},
    {359.1482, -853.3346, -74.47543, 5.654867},
    {374.9992, -879.0921, -75.56115, 1.867502},
    {339.5252, -850.4612, -74.45442, 4.764749},
    {337.0534, -864.002, -75.72749, 4.642576},
    {398.2797, -851.8694, -68.84419, 0.5759587},
}

local timers = {}

local mushroomDeque = {}

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

-- Threat-list random pick (the murmur / brundir idiom — the old
-- "no random-target SelectTarget bridge" claim is stale):
-- playersOnly honors C++ playerOnly via the GetObjectType
-- "Player" filter (illidan/murmur convention); maxDist <= 0 skips
-- the band (C++ dist 0.0f = any range). hasAuraExclude honors
-- the negative-aura SelectTarget filter (-SPELL_MINI, C++ :143).
local function randomThreatTarget(creature, maxDist, playersOnly, hasAuraExclude)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:IsAlive() and ((not playersOnly) or t:GetObjectType() == "Player") then
            local targetReach = 1.5
            if t:GetObjectType() ~= "Player" then
                targetReach = t:GetCombatReach()
            end
            if maxDist <= 0 or creature:GetDistance(t) - creature:GetCombatReach() - targetReach < maxDist then
                if hasAuraExclude == nil or not t:HasAura(hasAuraExclude) then
                    candidates[#candidates + 1] = t
                end
            end
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ SelectTarget(Random, 1, dist, true): UnitAI.h builds the list
-- victim-first, so offset 1 removes the victim, not the threat head;
-- then Player + alive + dist filters (eck precedent).
local function randomNonVictimTarget(creature, maxDist)
    local candidates = {}
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local targets = creature:GetAITargets()
    local aliveSeen = 0
    for _, t in ipairs(targets) do
        local skip = false
        if victimGuid ~= nil then
            skip = t:GetGUID() == victimGuid
        elseif not t:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and t:IsAlive() and t:GetObjectType() == "Player"
                and (maxDist <= 0 or creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < maxDist) then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_MINI (:141-148): SelectTarget(Random, 0, 0.0f, true,
-- true, -SPELL_MINI) — random alive player WITHOUT the Mini aura
-- (playerOnly=true, any range); on hit DoCastAOE(SPELL_MINI)
-- self-cast non-triggered (vaelastrasz/arugal convention) and
-- Repeat(30s); on miss Repeat(1s) fail-fast (the else branch, NOT
-- the 30s cycle).
local function miniTick(creature, guid)
    if randomThreatTarget(creature, 0, true, SPELL_MINI) ~= nil then
        creature:CastSpell(creature, SPELL_MINI)
        schedule(guid, "mini", 30000, function()
            miniTick(creature, guid)
        end)
    else
        schedule(guid, "mini", 1000, function()
            miniTick(creature, guid)
        end)
    end
end

-- C++ EVENT_ROOT (:150-153): SelectTarget(Random, 1, 100.0f, true)
-- -> DoCast(target, SPELL_ENTANGLING_ROOTS, TRIGGERED); the
-- Repeat(10s, 15s) sits outside the target check — fires even on
-- nil target (C++-exact).
local function rootTick(creature, guid)
    local target = randomNonVictimTarget(creature, 100.0)
    if target then
        creature:CastSpell(target, SPELL_ENTANGLING_ROOTS, true)
    end
    schedule(guid, "root", math.random(10000, 15000), function()
        rootTick(creature, guid)
    end)
end

-- C++ EVENT_BOLT (:155-161): SelectTarget(Random, 0, 100.0f,
-- true) — playerOnly=true (the 4th arg, not the 5th; the offset
-- sits at 0 so no tank skip) -> DoCast(target,
-- SPELL_VENOM_BOLT_VOLLEY, TRIGGERED); Repeat(18s, 22s) outside
-- the target check (C++-exact).
local function boltTick(creature, guid)
    local target = randomThreatTarget(creature, 100.0, true, nil)
    if target then
        creature:CastSpell(target, SPELL_VENOM_BOLT_VOLLEY, true)
    end
    schedule(guid, "bolt", math.random(18000, 22000), function()
        boltTick(creature, guid)
    end)
end

-- C++ EVENT_BASH: JustEngagedWith-scheduled 10-14s init,
-- rescheduled 7-12s; DoCastVictim. UpdateAI returns early while
-- casting — the Go engine handles cast-gating, the timer just
-- re-arms.
local function bashTick(creature, guid)
    creature:CastSpell(nil, SPELL_BASH)
    schedule(guid, "bash", math.random(7000, 12000), function()
        bashTick(creature, guid)
    end)
end

-- C++ EVENT_SPAWN (boss_amanitar.cpp:148-151, :130-133) — PORTED:
-- fires once, 1s after engage; for each of the 32 MushroomPositions,
-- SummonCreature(roll_chance_i(40) ? NPC_HEALTHY_MUSHROOM 30391 :
-- NPC_POISONOUS_MUSHROOM 30435, pos, TEMPSUMMON_CORPSE_TIMED_DESPAWN
-- Eluna type 5, 4s). The summoned mushrooms' Reset legs ride the
-- summoner ev19 below (amanitarJustSummoned); only the proximity
-- poison machine (MoveInLineOfSight) stays out.
local function spawnTick(creature, guid)
    for _, pos in ipairs(MUSHROOM_POSITIONS) do
        local entry = (math.random(1, 100) <= 40)
            and NPC_HEALTHY_MUSHROOM or NPC_POISONOUS_MUSHROOM
        creature:SummonCreature(entry, pos[1], pos[2], pos[3], pos[4], 5, 4000)
    end
end

-- C++ npc_amanitar_mushrooms::Reset (boss_amanitar.cpp:203-222) via the
-- summoner ev19 (svalna/anubarak precedent): a summoned mushroom's own
-- Reset never fires in Go (summonTempCreature fires no OnReset), so the
-- Reset legs land here at summon time. The stale "unbridged Modelid2
-- VALUE bearer" claim is retracted: GetTemplateModelId(2) is the bearer
-- (anubarak burrow precedent).
local function amanitarJustSummoned(_, creature, summoned)
    local entry = summoned:GetEntry()
    if entry ~= NPC_HEALTHY_MUSHROOM and entry ~= NPC_POISONOUS_MUSHROOM then
        return
    end
    summoned:SetReactState(0) -- REACT_PASSIVE
    summoned:SetDisplayId(summoned:GetTemplateModelId(2))
    summoned:CastSpell(summoned, SPELL_PUTRID_MUSHROOM)
    summoned:CastSpell(summoned, SPELL_SHRINK, true)
    summoned:CastSpell(summoned, SPELL_GROW, true)
    if entry == NPC_HEALTHY_MUSHROOM then
        summoned:CastSpell(summoned, SPELL_POWER_MUSHROOM_VISUAL_AURA)
    else
        summoned:CastSpell(summoned, SPELL_POISONOUS_MUSHROOM_VISUAL_AURA)
    end
    local sguid = summoned:GetGUID()
    schedule(sguid, "grow", 800, function()
        if not summoned:IsDead() then
            summoned:CastSpell(summoned, SPELL_GROW, true)
        end
    end)
end

-- C++ npc_amanitar_mushrooms::JustDied (boss_amanitar.cpp:248-252): a
-- healthy mushroom casts Potent Fungus on death. Entry-registered: the
-- kill funnel fires event 4 by entry for summoned creatures too.
local function mushroomDied(event, creature)
    if creature:GetEntry() == NPC_HEALTHY_MUSHROOM then
        creature:CastSpell(creature, SPELL_POTENT_FUNGUS, true)
    end
end

-- C++ boss_amanitar::SummonedCreatureDies (boss_amanitar.cpp:125-129) +
-- EVENT_RESPAWN (boss_amanitar.cpp:172-177): dead summons' positions queue
-- in _mushroomsDeque; every 40-60s the queue drains via SpawnMushroom
-- (40% healthy roll, type 5, 4s). ev21 args (event, creature, summon,
-- killer) per the anomalus precedent. The deque is deliberately NOT
-- cleared on leave/died/reset: C++ keeps it across evade (summons
-- despawn without dying), and EVENT_SPAWN re-fills the field on
-- re-engage.
local function amanitarSummonedDied(event, creature, summon, killer)
    local entry = summon:GetEntry()
    if entry ~= NPC_HEALTHY_MUSHROOM and entry ~= NPC_POISONOUS_MUSHROOM then
        return
    end
    local guid = creature:GetGUID()
    local deque = mushroomDeque[guid]
    if not deque then
        deque = {}
        mushroomDeque[guid] = deque
    end
    deque[#deque + 1] = {summon:GetX(), summon:GetY(), summon:GetZ(), summon:GetO()}
end

local function respawnTick(creature, guid)
    local deque = mushroomDeque[guid]
    if deque then
        for _, pos in ipairs(deque) do
            local entry = (math.random(1, 100) <= 40)
                and NPC_HEALTHY_MUSHROOM or NPC_POISONOUS_MUSHROOM
            creature:SummonCreature(entry, pos[1], pos[2], pos[3], pos[4], 5, 4000)
        end
        mushroomDeque[guid] = {}
    end
    schedule(guid, "respawn", math.random(40000, 60000), function()
        respawnTick(creature, guid)
    end)
end

local function amanitarEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ JustEngagedWith (boss_amanitar.cpp:96-101): EVENT_ROOT
    -- 5-9s, EVENT_BASH 10-14s, EVENT_BOLT 15-20s, EVENT_MINI
    -- 12-18s, EVENT_SPAWN 1s (PORTED — one-shot, above),
    -- EVENT_RESPAWN 40-60s (summon machine — unbridged).
    schedule(guid, "spawn", 1000, function()
        spawnTick(creature, guid)
    end)
    schedule(guid, "respawn", math.random(40000, 60000), function()
        respawnTick(creature, guid)
    end)
    schedule(guid, "root", math.random(5000, 9000), function()
        rootTick(creature, guid)
    end)
    schedule(guid, "bash", math.random(10000, 14000), function()
        bashTick(creature, guid)
    end)
    schedule(guid, "bolt", math.random(15000, 20000), function()
        boltTick(creature, guid)
    end)
    schedule(guid, "mini", math.random(12000, 18000), function()
        miniTick(creature, guid)
    end)
end

-- C++ instance->DoRemoveAurasDueToSpellOnPlayers(SPELL_MINI 57055, true,
-- true) player leg (boss_amanitar.cpp:107 EnterEvadeMode, :115 JustDied):
-- strip Mini from every player sharing the creature's map+instance —
-- GetPlayersInWorld + mapId/instanceId filter + player:RemoveAura (the
-- moroes precedent; C++-exact, no alive gate). The
-- includePets/includeControlled legs have no pet/controlled iteration
-- bridge — stay out.
local function stripMiniAuras(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId then
            p:RemoveAura(SPELL_MINI)
        end
    end
end

local function amanitarLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ EnterEvadeMode (boss_amanitar.cpp:103-109): summons.DespawnAll()
    -- (moot — nothing modeled) + instance DoRemoveAurasDueToSpellOnPlayers(
    -- SPELL_MINI 57055, true, true) — the player leg is PORTED via
    -- stripMiniAuras above (the includePets/includeControlled legs stay
    -- out — no pet/controlled iteration bridge) + _DespawnAtEvade() —
    -- DespawnOrUnsummon is immediate at evade (ScriptedCreature.cpp:596-616);
    -- drakkari precedent. Stale "no despawn bridge" retracted.
    stripMiniAuras(creature)
    creature:Despawn()
end

local function amanitarDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    -- C++ JustDied (boss_amanitar.cpp:111-116): _JustDied() is the
    -- cancelTimers above; DoCastAOE(SPELL_REMOVE_MUSHROOM_POWER)
    -- ports below; instance->DoRemoveAurasDueToSpellOnPlayers(
    -- SPELL_MINI 57055, true, true) — the player leg is PORTED via
    -- stripMiniAuras (the includePets/includeControlled legs stay out —
    -- no pet/controlled iteration bridge).
    creature:CastSpell(creature, SPELL_REMOVE_MUSHROOM_POWER)
    stripMiniAuras(creature)
end

local function amanitarReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_AMANITAR, 1, amanitarEnterCombat)
RegisterCreatureEvent(ENTRY_AMANITAR, 2, amanitarLeaveCombat)
RegisterCreatureEvent(ENTRY_AMANITAR, 4, amanitarDied)
RegisterCreatureEvent(ENTRY_AMANITAR, 23, amanitarReset)
RegisterCreatureEvent(ENTRY_AMANITAR, 19, amanitarJustSummoned)
RegisterCreatureEvent(ENTRY_AMANITAR, 21, amanitarSummonedDied)
RegisterCreatureEvent(NPC_HEALTHY_MUSHROOM, 4, mushroomDied)
RegisterCreatureEvent(NPC_POISONOUS_MUSHROOM, 4, mushroomDied)
