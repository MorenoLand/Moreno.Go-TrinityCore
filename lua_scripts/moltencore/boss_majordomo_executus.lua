-- Majordomo Executus (Molten Core) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/MoltenCore/
-- boss_majordomo_executus.cpp (boss_majordomoAI only); molten_core.h:38
-- (BOSS_MAJORDOMO_EXECUTUS = 8, ninth boss), :62
-- (NPC_MAJORDOMO_EXECUTUS = 12018).
-- Creature entry: 12018 Majordomo Executus (C++ ScriptName
-- "boss_majordomo" per AddSC_boss_majordomo). Talk lines used:
-- SAY_AGGRO=0 (pull), SAY_SLAY=2 (25% on kill), SAY_DEFEAT=4 (defeat
-- pump), SAY_SUMMON_MAJ=5 (gossip summon), SAY_ARRIVAL2_MAJ=6 (outro);
-- SAY_SPAWN=1 and SAY_SPECIAL=3 are declared in C++ but unused by the AI
-- (instance-side use).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Eluna gossip events: 2 OnGossipSelect.
-- Timers via CreateLuaEvent; melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat:
-- Talk(SAY_AGGRO), arm magic reflection 20619 30s then 30s,
-- non-triggered self-cast / damage reflection 21075 15s then 30s,
-- non-triggered self-cast / blast wave 20229 10s then 10s,
-- non-triggered DoCastVictim / teleport 20618 20s then 20s,
-- non-triggered on a threat-list target alive with the first
-- MaxThreat entry skipped BEFORE filters (C++ SelectTarget(Random, 1);
-- UnitAI.h offset-before-filter, not a victim-GUID exclusion; a
-- sole-entry tick casts nothing but keeps the schedule). Nil-
-- victim ticks cast nothing but keep the schedule (jeklik
-- convention). OnTargetDied(3): 25% Talk(SAY_SLAY). 1s aegis pump
-- (geddon convention): current health strictly below 50% (C++ HealthBelowPct(50)
-- reads current health; C++ re-casts every UpdateAI tick below 50%, no
-- once-guard) -> triggered self-cast Aegis of Ragnaros 20620. 1s defeat
-- pump: no Flamewaker Healer (11663) and no Flamewaker Elite (11664)
-- within 100 yd -> SetFaction(35 FRIENDLY) + Talk(SAY_DEFEAT) + combat
-- timers stop + 32s OUTRO_1 (NearTeleportTo RagnarosTelePos + gossip
-- flag). OnDied/OnLeaveCombat/OnReset: cancel timers. OnGossipSelect (DB menu 4108
-- option 0 — the defeated-majordomo "summon Ragnaros" menu): close
-- the menu, Talk(SAY_SUMMON_MAJ), arm the 8s Ragnaros summon + 24s
-- outro Talk(SAY_ARRIVAL2_MAJ).
-- The defeat sequence's remaining legs stay out: EnterEvadeMode has no
-- bridge, and the instance DONE / _JustDied legs have no bridge. The
-- flamewakers are instance-summoned (instance_molten_core.cpp
-- SummonMajordomoExecutus), but the presence check itself runs on the
-- bridged FindNearestCreature gate, so the trigger + visible legs
-- (faction, Talk, OUTRO_1 teleport + gossip flag) are PORTED below.
-- EVENT_OUTRO_2 + the DoAction(ACTION_START_RAGNAROS)
-- arms are PORTED in the gossip handler below (RemoveFlag, Talk,
-- 8s Ragnaros summon, 24s Talk). DoAction(ACTION_START_RAGNAROS_ALT)
-- stays out (instance-summon entry point, no bearer); SPELL_SUMMON_
-- RAGNAROS 19774 is declared but unused by the AI.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no live UNIT_STATE_CASTING read bridge (IsCasting is constant-false — Go casts are fire-and-forget) — timers
-- fire unconditionally (jeklik convention); no instance-script model
-- — boss admission via the luaBossAI shim (BossAI::JustEngagedWith
-- and the BOSS_MAJORDOMO_EXECUTUS bookkeeping arms skipped).

local ENTRY_MAJORDOMO_EXECUTUS = 12018
local NPC_RAGNAROS = 11502

local SAY_AGGRO = 0
local SAY_SLAY = 2
local SAY_DEFEAT = 4
local SAY_SUMMON_MAJ = 5
local SAY_ARRIVAL2_MAJ = 6

local SPELL_BLAST_WAVE = 20229
local SPELL_TELEPORT = 20618
local SPELL_MAGIC_REFLECTION = 20619
local SPELL_AEGIS_OF_RAGNAROS = 20620
local SPELL_DAMAGE_REFLECTION = 21075

local ENTRY_FLAMEWAKER_HEALER = 11663
local ENTRY_FLAMEWAKER_ELITE = 11664
local FACTION_FRIENDLY = 35
local UNIT_NPC_FLAG_GOSSIP = 1
local RAGNAROS_TELE_X = 829.159
local RAGNAROS_TELE_Y = -815.773
local RAGNAROS_TELE_Z = -228.972
local RAGNAROS_TELE_O = 5.305

local timers = {}
local defeated = {}

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

-- C++ EVENT_TELEPORT: non-triggered DoCast on a threat-list target
-- alive with the victim skipped BEFORE filters
-- (SelectTarget(Random, 1); UnitAI.h builds the list victim-first,
-- so offset 1 pops the victim — akilzon idiom); a sole-entry tick
-- casts nothing. Re-arm 20s regardless (C++ re-arms unconditionally).
local function randomThreatTargetSkipFirst(creature)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local targets = creature:GetAITargets()
    local out = {}
    local aliveSeen = 0
    for _, unit in ipairs(targets) do
        local skip = false
        if victimGuid ~= nil then
            skip = unit:GetGUID() == victimGuid
        elseif not unit:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and not unit:IsDead() then
            out[#out + 1] = unit
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
end

local function onTeleport(creature, guid)
    local pick = randomThreatTargetSkipFirst(creature)
    if pick then
        creature:CastSpell(pick, SPELL_TELEPORT)
    end
    schedule(guid, "teleport", 20000, function()
        onTeleport(creature, guid)
    end)
end

-- C++ UpdateAI defeat arm (boss_majordomo_executus.cpp:108-124): while
-- boss state != DONE (unmodeled — the pump only runs in combat, and the
-- combat timers die with it), if no Flamewaker Healer (11663) and no
-- Flamewaker Elite (11664) is found within 100 yd -> SetFaction(35
-- FRIENDLY) + Talk(SAY_DEFEAT) + combat timers stop + EVENT_OUTRO_1
-- at 32s (NearTeleportTo RagnarosTelePos + gossip flag). EnterEvadeMode,
-- instance DONE and _JustDied legs have no bridge and stay out.
local function onDefeatPump(creature, guid)
    if defeated[guid] then
        return
    end
    if creature:FindNearestCreature(ENTRY_FLAMEWAKER_HEALER, 100.0) == nil
            and creature:FindNearestCreature(ENTRY_FLAMEWAKER_ELITE, 100.0) == nil then
        defeated[guid] = true
        cancelTimers(guid)
        creature:SetFaction(FACTION_FRIENDLY)
        creature:Talk(SAY_DEFEAT)
        schedule(guid, "outro1", 32000, function()
            creature:Teleport(RAGNAROS_TELE_X, RAGNAROS_TELE_Y, RAGNAROS_TELE_Z, RAGNAROS_TELE_O)
            creature:SetFlag(82, UNIT_NPC_FLAG_GOSSIP)
        end)
        return
    end
    schedule(guid, "defeat", 1000, function()
        onDefeatPump(creature, guid)
    end)
end

-- Forward declaration: majordomoEnterCombat's timer closures reference
-- onAegisPump textually before its local definition; without this the
-- name compiles to a global and the aegis pump stream calls nil.
local onAegisPump

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
    schedule(guid, "aegis", 1000, function()
        onAegisPump(creature, guid)
    end)
    schedule(guid, "defeat", 1000, function()
        onDefeatPump(creature, guid)
    end)
end

-- C++ KilledUnit: 25% Talk(SAY_SLAY) (rand32() % 100 < 25,
-- C++-exact).
local function majordomoTargetDied(event, creature, victim)
    if math.random(0, 99) < 25 then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ UpdateAI: HealthBelowPct(50) -> triggered self-cast Aegis of
-- Ragnaros 20620 — checked every tick (no once-guard, no DamageTaken
-- hook in C++). Modeled as a 1s gated pump (geddon convention);
-- strictly below the floored threshold (ironaya convention).
-- NOTE: plain assignment, NOT `local function` — the forward-declared
-- `local onAegisPump` above is a separate slot; re-declaring with
-- `local function` would shadow it and the pump would call nil.
onAegisPump = function(creature, guid)
    local maxHealth = creature:GetMaxHealth()
    if maxHealth ~= 0
            and creature:GetHealth() < math.floor(maxHealth * 50 / 100) then
        creature:CastSpell(creature, SPELL_AEGIS_OF_RAGNAROS, true)
    end
    schedule(guid, "aegis", 1000, function()
        onAegisPump(creature, guid)
    end)
end

local function majordomoResetState(guid)
    defeated[guid] = nil
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
-- DoAction(ACTION_START_RAGNAROS): RemoveFlag(UNIT_NPC_FLAGS,
-- UNIT_NPC_FLAG_GOSSIP) -> Talk(SAY_SUMMON_MAJ) -> EVENT_OUTRO_2 8s
-- (instance map-summon at RagnarosSummonPos, duration 0 -> DEAD_DESPAWN,
-- Go summonType 6) -> EVENT_OUTRO_3 24s (Talk(SAY_ARRIVAL2_MAJ)).
-- C++ summoner is the map (no summoner); the Go bridge stamps majordomo
-- as owner — Ragnaros's Lua never reads its summoner, so no observable
-- difference. The hello menu itself is DB-side (menu 4108); only the
-- select arm is modeled.
local function majordomoGossipSelect(event, player, creature, sender, action, code, listID)
    -- C++ OnGossipSelect gates on menuId == 4108 (the DB gossip menu)
    -- && gossipListId == 0 (the challenge option — the menu's only
    -- option). The creature select hook now carries the trailing
    -- listID arg (gossip.go), so listID == 0 IS the C++ gate; the
    -- sender == 0 check is retained as belt-and-braces (DB items
    -- carry sender 0). The old `action ~= 0` gate compared the DB
    -- option's Action field (OptionType, never 0 for a visible
    -- option) against the gossipListId — it could never pass, leaving
    -- the whole outro arm dead in-game.
    if sender ~= 0 or listID ~= 0 then
        return false
    end
    player:GossipClearMenu()
    player:GossipComplete()
    local guid = creature:GetGUID()
    creature:RemoveFlag(82, 1)
    creature:Talk(SAY_SUMMON_MAJ)
    schedule(guid, "outro2", 8000, function()
        creature:SummonCreature(NPC_RAGNAROS, 838.510, -829.840, -232.0, 2.0, 6, 0)
    end)
    schedule(guid, "outro3", 24000, function()
        creature:Talk(SAY_ARRIVAL2_MAJ)
    end)
    -- C++ OnGossipSelect returns false: the AI/native select arms
    -- proceed (Eluna GOSSIP_EVENT_ON_SELECT: a Lua false lets them
    -- proceed; true/nothing would veto them).
    return false
end

RegisterCreatureEvent(ENTRY_MAJORDOMO_EXECUTUS, 1, majordomoEnterCombat)
RegisterCreatureEvent(ENTRY_MAJORDOMO_EXECUTUS, 2, majordomoLeaveCombat)
RegisterCreatureEvent(ENTRY_MAJORDOMO_EXECUTUS, 3, majordomoTargetDied)
RegisterCreatureEvent(ENTRY_MAJORDOMO_EXECUTUS, 4, majordomoDied)
RegisterCreatureEvent(ENTRY_MAJORDOMO_EXECUTUS, 23, majordomoReset)
RegisterCreatureGossipEvent(ENTRY_MAJORDOMO_EXECUTUS, 2, majordomoGossipSelect)
