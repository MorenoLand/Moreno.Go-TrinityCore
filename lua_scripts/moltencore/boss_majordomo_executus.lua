-- Majordomo Executus (Molten Core) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/MoltenCore/
-- boss_majordomo_executus.cpp (boss_majordomoAI only); molten_core.h:38
-- (BOSS_MAJORDOMO_EXECUTUS = 8, ninth boss), :62
-- (NPC_MAJORDOMO_EXECUTUS = 12018).
-- Creature entry: 12018 Majordomo Executus (C++ ScriptName
-- "boss_majordomo" per AddSC_boss_majordomo). Talk lines used:
-- SAY_AGGRO=0 (pull), SAY_SLAY=2 (25% on kill), SAY_SUMMON_MAJ=5
-- (gossip summon), SAY_ARRIVAL2_MAJ=6 (outro); SAY_DEFEAT=4 belongs
-- to the unmodeled defeat arm (see deviations); SAY_SPAWN=1 and
-- SAY_SPECIAL=3 are declared in C++ but unused by the AI
-- (instance-side use).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken (pre-damage hook, moroes
-- convention), 23 OnReset. Eluna gossip events: 2 OnGossipSelect.
-- Timers via CreateLuaEvent; melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat:
-- Talk(SAY_AGGRO), arm magic reflection 20619 30s then 30s,
-- non-triggered self-cast / damage reflection 21075 15s then 30s,
-- non-triggered self-cast / blast wave 20229 10s then 10s,
-- non-triggered DoCastVictim / teleport 20618 20s then 20s,
-- non-triggered on a random alive player in the instance excluding
-- the current victim (C++ SelectTarget(Random, 1) skips position 0;
-- a sole-victim tick casts nothing but keeps the schedule). Nil-
-- victim ticks cast nothing but keep the schedule (jeklik
-- convention). OnTargetDied(3): 25% Talk(SAY_SLAY). OnDamageTaken(9,
-- pre-damage hook): current (pre-damage) health strictly below 50%
-- (C++ HealthBelowPct(50) reads current health; the engine's event 9
-- fires before damage is applied, matching C++ Unit::DealDamage) ->
-- triggered self-cast Aegis of Ragnaros 20620 (C++-exact — no
-- once-guard; C++ re-casts every UpdateAI tick below 50%). OnDied/
-- OnLeaveCombat/OnReset: cancel timers. OnGossipSelect (DB menu 4108
-- option 0 — the defeated-majordomo "summon Ragnaros" menu): close
-- the menu, Talk(SAY_SUMMON_MAJ), arm the 24s outro
-- Talk(SAY_ARRIVAL2_MAJ).
-- The defeat/outro arms have no bridges and are not modeled: the
-- no-flamewaker defeat trigger (no nearest-creature lookup — healer
-- 11663 / elite 11664), the SetFaction(FACTION_FRIENDLY) arm, the
-- EnterEvadeMode + _JustDied defeat sequence, EVENT_OUTRO_1's
-- NearTeleportTo(RagnarosTelePos) + gossip-flag arm (no teleport/
-- flag bridges), EVENT_OUTRO_2's SummonCreature(NPC_RAGNAROS 11502)
-- arm (no summon bridge), and the DoAction(ACTION_START_RAGNAROS /
-- ACTION_START_RAGNAROS_ALT) arms (no DoAction bearer — the instance
-- script is blocked on the instance-script model); SPELL_SUMMON_
-- RAGNAROS 19774 is declared but unused by the AI.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no UNIT_STATE bridge — timers
-- fire unconditionally (jeklik convention); no instance-script model
-- — boss admission via the luaBossAI shim (BossAI::JustEngagedWith
-- and the BOSS_MAJORDOMO_EXECUTUS bookkeeping arms skipped).

local ENTRY_MAJORDOMO_EXECUTUS = 12018

local SAY_AGGRO = 0
local SAY_SLAY = 2
local SAY_SUMMON_MAJ = 5
local SAY_ARRIVAL2_MAJ = 6

local SPELL_BLAST_WAVE = 20229
local SPELL_TELEPORT = 20618
local SPELL_MAGIC_REFLECTION = 20619
local SPELL_AEGIS_OF_RAGNAROS = 20620
local SPELL_DAMAGE_REFLECTION = 21075

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

-- C++ EVENT_MAGIC_REFLECTION: non-triggered DoCast(me, 20619);
-- re-arm 30s.
local function onMagicReflection(creature, guid)
    creature:CastSpell(creature, SPELL_MAGIC_REFLECTION)
    schedule(guid, "magicreflection", 30000, function()
        onMagicReflection(creature, guid)
    end)
end

-- C++ EVENT_DAMAGE_REFLECTION: non-triggered DoCast(me, 21075);
-- re-arm 30s.
local function onDamageReflection(creature, guid)
    creature:CastSpell(creature, SPELL_DAMAGE_REFLECTION)
    schedule(guid, "damagereflection", 30000, function()
        onDamageReflection(creature, guid)
    end)
end

-- C++ EVENT_BLAST_WAVE: non-triggered DoCastVictim(20229);
-- re-arm 10s.
local function onBlastWave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BLAST_WAVE)
    end
    schedule(guid, "blastwave", 10000, function()
        onBlastWave(creature, guid)
    end)
end

-- C++ EVENT_TELEPORT: non-triggered DoCast on a random alive player
-- in the instance excluding the current victim (SelectTarget(Random,
-- 1) skips position 0); a sole-victim tick casts nothing. Re-arm
-- 20s regardless (C++ re-arms unconditionally).
local function randomAlivePlayerExcluding(creature, victim)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local victimGUID = victim and victim:GetGUID() or 0
    local players = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and p:GetGUID() ~= victimGUID then
            players[#players + 1] = p
        end
    end
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

local function onTeleport(creature, guid)
    local pick = randomAlivePlayerExcluding(creature, creature:GetVictim())
    if pick then
        creature:CastSpell(pick, SPELL_TELEPORT)
    end
    schedule(guid, "teleport", 20000, function()
        onTeleport(creature, guid)
    end)
end

local function majordomoEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "magicreflection", 30000, function()
        onMagicReflection(creature, guid)
    end)
    schedule(guid, "damagereflection", 15000, function()
        onDamageReflection(creature, guid)
    end)
    schedule(guid, "blastwave", 10000, function()
        onBlastWave(creature, guid)
    end)
    schedule(guid, "teleport", 20000, function()
        onTeleport(creature, guid)
    end)
end

-- C++ KilledUnit: 25% Talk(SAY_SLAY) (rand32() % 100 < 25,
-- C++-exact).
local function majordomoTargetDied(event, creature, victim)
    if math.random(0, 99) < 25 then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ UpdateAI: current (pre-damage) health strictly below 50%
-- (HealthBelowPct(50) — the engine's event 9 fires before damage is
-- applied, matching C++ Unit::DealDamage) -> triggered self-cast
-- Aegis of Ragnaros 20620 (C++-exact — no once-guard; C++ re-casts
-- every UpdateAI tick below 50%, so the damage hook re-casts per
-- damage event below 50%, geddon convention).
local function majordomoDamageTaken(event, creature, attacker, damage)
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if creature:GetHealth() * 100 / maxHealth < 50 then
        creature:CastSpell(creature, SPELL_AEGIS_OF_RAGNAROS, true)
    end
end

local function majordomoResetState(guid)
    cancelTimers(guid)
end

local function majordomoLeaveCombat(event, creature)
    majordomoResetState(creature:GetGUID())
end

local function majordomoDied(event, creature, killer)
    majordomoResetState(creature:GetGUID())
end

local function majordomoReset(event, creature)
    majordomoResetState(creature:GetGUID())
end

-- C++ OnGossipSelect: menu 4108 option 0 -> CloseGossipMenuFor,
-- DoAction(ACTION_START_RAGNAROS): Talk(SAY_SUMMON_MAJ), arm
-- EVENT_OUTRO_2 8s (summon — no bridge) and EVENT_OUTRO_3 24s
-- (Talk(SAY_ARRIVAL2_MAJ)). The gossip-flag RemoveFlag arm has no
-- bridge. The hello menu itself is DB-side (menu 4108); only the
-- select arm is modeled.
local function majordomoGossipSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    player:GossipComplete()
    local guid = creature:GetGUID()
    creature:Talk(SAY_SUMMON_MAJ)
    schedule(guid, "outro3", 24000, function()
        creature:Talk(SAY_ARRIVAL2_MAJ)
    end)
end

RegisterCreatureEvent(ENTRY_MAJORDOMO_EXECUTUS, 1, majordomoEnterCombat)
RegisterCreatureEvent(ENTRY_MAJORDOMO_EXECUTUS, 2, majordomoLeaveCombat)
RegisterCreatureEvent(ENTRY_MAJORDOMO_EXECUTUS, 3, majordomoTargetDied)
RegisterCreatureEvent(ENTRY_MAJORDOMO_EXECUTUS, 4, majordomoDied)
RegisterCreatureEvent(ENTRY_MAJORDOMO_EXECUTUS, 9, majordomoDamageTaken)
RegisterCreatureEvent(ENTRY_MAJORDOMO_EXECUTUS, 23, majordomoReset)
RegisterCreatureGossipEvent(ENTRY_MAJORDOMO_EXECUTUS, 2, majordomoGossipSelect)
