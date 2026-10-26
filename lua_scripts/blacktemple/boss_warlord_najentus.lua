-- High Warlord Naj'entus (Black Temple) — Lua port of
-- src/server/scripts/Outland/BlackTemple/boss_warlord_najentus.cpp
-- (boss_najentus only; go_najentus_spine documented below, not
-- registered); black_temple.h:31 (DATA_HIGH_WARLORD_NAJENTUS = 0,
-- first boss), :71 (NPC_HIGH_WARLORD_NAJENTUS = 22887). Creature
-- entry: 22887 High Warlord Naj'entus (C++ ScriptName
-- "boss_najentus" per AddSC_boss_najentus). Talk lines used:
-- SAY_AGGRO=0 (pull), SAY_NEEDLE=1 (impaling spine), SAY_SLAY=2
-- (kill, player-only), SAY_SPECIAL=3 (yell), SAY_ENRAGE=4
-- (berserk), SAY_DEATH=5 (death).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 14 OnHitBySpell (SpellHit), 23 OnReset.
-- Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat:
-- Talk(SAY_AGGRO), arm needle spine targeting 39992 2s then 2s,
-- triggered self-cast / tidal shield 39872 60s then {55s,60s},
-- triggered self-cast / impaling spine 39837 30s then {20s,25s},
-- triggered on a random alive player in the instance within 200 yd
-- excluding the current victim (C++ SelectTarget(Random, 1, 200.0f)
-- skips position 0; a nil pick casts nothing but keeps the
-- schedule — jeklik convention) + Talk(SAY_NEEDLE) / berserk 26662
-- 480s, triggered self-cast + Talk(SAY_ENRAGE) / yell {45s,100s}
-- then {25s,100s}, Talk(SAY_SPECIAL). The shield tick re-arms the
-- spine timer at 50s (C++ RescheduleEvent overwrites the pending
-- spine timer; re-arms 2s on the tidal-burst pop — see SpellHit).
-- OnTargetDied(3): Talk(SAY_SLAY), TYPEID_PLAYER gate (terestian
-- convention). OnDied(4): Talk(SAY_DEATH). OnLeaveCombat(2)/
-- OnReset(23): cancel timers (C++ Reset clears _spineTargetGUID —
-- the GUID itself is unmodeled, see deviations). The C++ shield
-- tick's RescheduleEvent(EVENT_SPINE, 50s) + Repeat(55s,60s) is
-- modeled by canceling and re-arming the spine timer; the per-tick
-- re-arm helpers below mirror the terestian SpellHit re-arm shape.
-- The go_najentus_spine GameObjectAI in the same C++ file (GO 185584
-- per black_temple.h:119; the gossip arm calls instance GetCreature
-- (DATA_HIGH_WARLORD_NAJENTUS) -> GetData(DATA_REMOVE_IMPALING_
-- SPINE) -> DoAction(ACTION_RESET_IMPALING_TARGET), then casts 39956
-- triggered on the player and deletes the GO) is not registered —
-- the instance-creature lookup is blocked on the instance-script
-- model, so the boss's GetData/DoAction arms (RemoveImpalingSpine,
-- _spineTargetGUID) and the impaling-spine removal itself are
-- unmodeled (standing instance-script gap). The needle-spine-
-- targeting SpellScript (filters impaling-spine targets from the
-- 39992 area pick, casts 39835 needle spine via dummy effect) has
-- no SpellScript bridge — the targeting filter and the dummy cast
-- are unmodeled (standing SpellScript gap).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate
-- and the post-event casting check have no live UNIT_STATE_CASTING read bridge (IsCasting is constant-false — Go casts are fire-and-forget) —
-- timers fire unconditionally (jeklik convention); no instance-
-- script model — boss admission via the luaBossAI shim
-- (BossAI::JustEngagedWith and the DATA_HIGH_WARLORD_NAJENTUS
-- bookkeeping arms skipped); the impaling-spine GO summon (185584)
-- is ported via creature:SummonGameObject in onSpine (the Summon
-- bridge landed after the old "no summon bridge" note was written);
-- the EnterEvadeMode _DespawnAtEvade arm rides creature:Despawn in
-- najentusLeaveCombat (drakkari/moorabi precedent).

local ENTRY_NAJENTUS = 22887

local SAY_AGGRO = 0
local SAY_NEEDLE = 1
local SAY_SLAY = 2
local SAY_SPECIAL = 3
local SAY_ENRAGE = 4
local SAY_DEATH = 5

local SPELL_NEEDLE_SPINE_TARGETING = 39992
local SPELL_IMPALING_SPINE = 39837
local SPELL_TIDAL_BURST = 39878
local SPELL_TIDAL_SHIELD = 39872
local SPELL_HURL_SPINE = 39948
local SPELL_BERSERK = 26662

local GO_NAJENTUS_SPINE = 185584

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

local function cancelTimer(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

-- C++ SelectTarget(Random, 1, 200.0f, true): threat-list players
-- alive within 200 yd, minus the victim (UnitAI.h builds the list
-- victim-first, so offset 1 removes the victim, not the threat head).
local function randomSpineTarget(creature)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end

    local targets = creature:GetAITargets()
    local candidates = {}
    for i, p in ipairs(targets) do
        if ((victimGuid ~= nil and p:GetGUID() ~= victimGuid)
                or (victimGuid == nil and i > 1))
                and p:GetObjectType() == "Player" and not p:IsDead()
                and creature:GetDistance(p) <= 200 then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_NEEDLE: triggered self-cast 39992; re-arm 2s.
local function onNeedle(creature, guid)
    creature:CastSpell(creature, SPELL_NEEDLE_SPINE_TARGETING, true)
    schedule(guid, "needle", 2000, function()
        onNeedle(creature, guid)
    end)
end

-- C++ EVENT_SPINE (boss_warlord_najentus.cpp:152-160), arm order:
-- triggered 39837 on the random spine target, _spineTargetGUID =
-- target [unmodeled — removal machine is instance-bridged], the
-- target summons GO_NAJENTUS_SPINE 185584 at its own position
-- (30s, QuaternionData cosmetic-dropped) — ported on the creature
-- (target position via the bridged GetX/GetY/GetZ/GetO fallback;
-- summoner-owner tracking has no Lua-visible effect) — then
-- Talk(SAY_NEEDLE); re-arm {20s,25s} unconditional (C++-exact —
-- the repeat runs even when no target was found).
local function onSpine(creature, guid)
    local target = randomSpineTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_IMPALING_SPINE, true)
        creature:SummonGameObject(GO_NAJENTUS_SPINE, target:GetX(), target:GetY(), target:GetZ(), target:GetO(), 30)
        creature:Talk(SAY_NEEDLE)
    end
    schedule(guid, "spine", 20000 + math.random(0, 5000), function()
        onSpine(creature, guid)
    end)
end

-- C++ EVENT_SHIELD: triggered self-cast 39872; re-arm the spine
-- timer at 50s; repeat {55s,60s}.
local function onShield(creature, guid)
    creature:CastSpell(creature, SPELL_TIDAL_SHIELD, true)
    cancelTimer(guid, "spine")
    schedule(guid, "spine", 50000, function()
        onSpine(creature, guid)
    end)
    schedule(guid, "shield", 55000 + math.random(0, 5000), function()
        onShield(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: Talk(SAY_ENRAGE) + triggered self-cast 26662;
-- one-shot (C++-exact — no re-arm).
local function onBerserk(creature, guid)
    creature:Talk(SAY_ENRAGE)
    creature:CastSpell(creature, SPELL_BERSERK, true)
end

-- C++ EVENT_YELL: Talk(SAY_SPECIAL); re-arm {25s,100s}.
local function onYell(creature, guid)
    creature:Talk(SAY_SPECIAL)
    schedule(guid, "yell", 25000 + math.random(0, 75000), function()
        onYell(creature, guid)
    end)
end

local function najentusEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "needle", 2000, function()
        onNeedle(creature, guid)
    end)
    schedule(guid, "shield", 60000, function()
        onShield(creature, guid)
    end)
    schedule(guid, "spine", 30000, function()
        onSpine(creature, guid)
    end)
    schedule(guid, "berserk", 480000, function()
        onBerserk(creature, guid)
    end)
    schedule(guid, "yell", 45000 + math.random(0, 55000), function()
        onYell(creature, guid)
    end)
end

local function najentusResetState(guid)
    cancelTimers(guid)
end

local function najentusLeaveCombat(event, creature)
    najentusResetState(creature:GetGUID())
    -- C++ EnterEvadeMode: _EnterEvadeMode() + _DespawnAtEvade()
    -- (boss_warlord_najentus.cpp:77-80).
    creature:Despawn()
end

-- C++ KilledUnit: Talk(SAY_SLAY), TYPEID_PLAYER gate.
local function najentusTargetDied(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(SAY_DEATH).
local function najentusDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    najentusResetState(creature:GetGUID())
end

local function najentusReset(event, creature)
    najentusResetState(creature:GetGUID())
end

-- C++ SpellHit: hit by 39948 (hurl spine) while carrying 39872
-- (tidal shield) -> remove the shield, triggered self-cast 39878
-- (tidal burst), re-arm EVENT_SPINE at 2s (terestian SpellHit
-- convention — re-arm overwrites the pending spine timer).
local function najentusSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_HURL_SPINE
            and creature:HasAura(SPELL_TIDAL_SHIELD) then
        creature:RemoveAura(SPELL_TIDAL_SHIELD)
        creature:CastSpell(creature, SPELL_TIDAL_BURST, true)
        local guid = creature:GetGUID()
        cancelTimer(guid, "spine")
        schedule(guid, "spine", 2000, function()
            onSpine(creature, guid)
        end)
    end
end

RegisterCreatureEvent(ENTRY_NAJENTUS, 1, najentusEnterCombat)
RegisterCreatureEvent(ENTRY_NAJENTUS, 2, najentusLeaveCombat)
RegisterCreatureEvent(ENTRY_NAJENTUS, 3, najentusTargetDied)
RegisterCreatureEvent(ENTRY_NAJENTUS, 4, najentusDied)
RegisterCreatureEvent(ENTRY_NAJENTUS, 14, najentusSpellHit)
RegisterCreatureEvent(ENTRY_NAJENTUS, 23, najentusReset)
