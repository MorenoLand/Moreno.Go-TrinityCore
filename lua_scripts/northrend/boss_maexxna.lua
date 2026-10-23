-- Maexxna (Naxxramas, Arachnid Quarter) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_maexxna.cpp
-- (boss_maexxnaAI : public BossAI).
-- Entry 15952 — "Maexxna" (wowhead-verified:
-- wowhead.com/?npc=15952 "Maexxna - NPC - World of Warcraft"; no NPC_
-- constant in the C++ tree — naxxramas.h carries only the BOSS_MAEXXNA
-- data ID, so the creature entry is DB-side like flamelash's 9156).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent per-GUID timers (cancel on
-- leave combat / died / reset); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms):
-- OnEnterCombat(1): EVENT_SPRAY 40s, EVENT_SHOCK randtime(5s,10s),
-- EVENT_POISON randtime(10s,15s), EVENT_SUMMON 30s.
-- spray arm — Talk(EMOTE_WEB_SPRAY 2) + DoCastAOE(SPELL_WEB_SPRAY
-- 29484); DoCastAOE lowers to creature:CastSpell(creature, spell)
-- (DoCastAOE self-cast convention); repeat 40s.
-- shock arm — DoCastAOE(SPELL_POISON_SHOCK 28741); repeat
-- randtime(10s,20s).
-- poison arm — DoCastVictim(SPELL_NECROTIC_POISON 28776);
-- creature:CastSpell(nil, spell) = victim-cast (vaelastrasz
-- convention); repeat randtime(10s,20s).
-- summon arm — Talk(EMOTE_SPIDERS 0) is unconditional in C++; the
-- tick keeps the C++-exact 30s/40s schedule with the PORTED
-- DoSummon(NPC_SPIDERLING 17055) x8-10 leg (Eluna type 5,
-- 5s; the stale "no summon bridge" claim is retracted).
-- frenzy latch — C++ UpdateAI checks every tick: HealthBelowPct(30)
-- && !me->HasAura(SPELL_FRENZY_HELPER); checked by a 1s per-GUID
-- pump (azuregos enrage-pump convention). SPELL_FRENZY_HELPER is
-- RAID_MODE(54123, 54124) — no difficulty bridge, so both aura IDs
-- are checked (faerlina RAID_MODE precedent); the cast is DoCast(54123)
-- i.e. self-cast (ScriptedAI::DoCast without a target casts on self).
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23): cancel the timers. The C++
-- Reset() also runs instance->DoRemoveAurasDueToSpellOnPlayers(
-- SPELL_WEB_WRAP 28622) — no instance bridge (standing).
-- Deliberate deviations (all await engine bridges): EVENT_WRAP
-- (DoSummon(NPC_WEB_WRAP 16486) + wrap->AI()->SetGUID + target->
-- MoveJump + MoveSplineInit/web-wrap aura application) — no summon /
-- SetGUID-cross-AI / MoveJump / MoveSplineInit bridges (frogger
-- precedent); Talk(EMOTE_WEB_WRAP 1) is target-conditional in C++
-- and stays out with the wrap leg rather than fabricate a lone emote;
-- npc_webwrap (CreatureScript "npc_webwrap", entry 16486) is the
-- summon-side AI of that same machine; only its JustDied
-- DespawnOrUnsummon(5s) leg ports (event 4, below — the freya
-- delayed-despawn idiom); the victimGUID aura-removal leg has no
-- bridge (stays out).

local SPELL_WEB_SPRAY = 29484
local SPELL_POISON_SHOCK = 28741
local SPELL_NECROTIC_POISON = 28776
local SPELL_FRENZY = 54123
local SPELL_FRENZY_HELPER_25 = 54124

local EMOTE_SPIDERS = 0
local EMOTE_WEB_SPRAY = 2

-- boss_maexxna.cpp enum (local, no DB-side creature template).
local NPC_SPIDERLING = 17055

local ENTRY = 15952
local ENTRY_WEB_WRAP = 16486 -- boss_maexxna.cpp:47

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

local function sprayTick(creature, guid)
    creature:Talk(EMOTE_WEB_SPRAY)
    creature:CastSpell(creature, SPELL_WEB_SPRAY)
    schedule(guid, "spray", 40000, function()
        sprayTick(creature, guid)
    end)
end

local function shockTick(creature, guid)
    creature:CastSpell(creature, SPELL_POISON_SHOCK)
    schedule(guid, "shock", math.random(10000, 20000), function()
        shockTick(creature, guid)
    end)
end

local function poisonTick(creature, guid)
    creature:CastSpell(nil, SPELL_NECROTIC_POISON)
    schedule(guid, "poison", math.random(10000, 20000), function()
        poisonTick(creature, guid)
    end)
end

local function summonTick(creature, guid)
    -- PORTED (boss_maexxna.cpp:179-184): Talk(EMOTE_SPIDERS) then
    -- urand(8,10) x DoSummon(NPC_SPIDERLING 17055, me, 4.0f,
    -- TEMPSUMMON_CORPSE_TIMED_DESPAWN Eluna type 5, 5s); the
    -- random-near-4.0f is a uniform polar offset around the boss.
    -- Delta: summoned spiderling AI is unported — summons are inert.
    creature:Talk(EMOTE_SPIDERS)
    local cx, cy, cz = creature:GetX(), creature:GetY(), creature:GetZ()
    for _ = 1, math.random(8, 10) do
        local angle = math.random() * math.pi * 2
        local r = math.random() * 4
        creature:SummonCreature(NPC_SPIDERLING, cx + r * math.cos(angle),
            cy + r * math.sin(angle), cz, 0, 5, 5000)
    end
    schedule(guid, "summon", 40000, function()
        summonTick(creature, guid)
    end)
end

local function frenzyPump(creature, guid)
    local maxHealth = creature:GetMaxHealth()
    if maxHealth > 0
            and creature:GetHealth() < math.floor(maxHealth * 30 / 100)
            and not creature:HasAura(SPELL_FRENZY)
            and not creature:HasAura(SPELL_FRENZY_HELPER_25) then
        creature:CastSpell(creature, SPELL_FRENZY)
    end
    schedule(guid, "frenzy", 1000, function()
        frenzyPump(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- boss_maexxna.cpp JustEngagedWith schedule.
    schedule(guid, "spray", 40000, function()
        sprayTick(creature, guid)
    end)
    schedule(guid, "shock", math.random(5000, 10000), function()
        shockTick(creature, guid)
    end)
    schedule(guid, "poison", math.random(10000, 15000), function()
        poisonTick(creature, guid)
    end)
    schedule(guid, "summon", 30000, function()
        summonTick(creature, guid)
    end)
    schedule(guid, "frenzy", 1000, function()
        frenzyPump(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)

-- C++ npc_webwrapAI::JustDied (boss_maexxna.cpp:243-250): the victim
-- aura-removal leg rides the victimGUID member (no bridge, stays out);
-- me->DespawnOrUnsummon(5s) ports via the freya delayed-despawn idiom
-- (CreateLuaEvent one-shot).
local function webWrapJustDied(event, creature, killer)
    CreateLuaEvent(function()
        creature:Despawn()
    end, 5000)
end

RegisterCreatureEvent(ENTRY_WEB_WRAP, 4, webWrapJustDied)
RegisterCreatureEvent(ENTRY_WEB_WRAP, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- boss_maexxna.cpp:228: custom UpdateAI never calls DoMeleeAttackIfReady (visibility-timer UpdateAI in C++, never melees)
end)
