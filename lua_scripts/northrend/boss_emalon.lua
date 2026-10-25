-- Emalon the Storm Watcher (Vault of Archavon) — Lua port of
-- src/server/scripts/Northrend/VaultOfArchavon/boss_emalon.cpp
-- (290 lines; 2 scripts — boss_emalon and npc_tempest_minion, both
-- registered from inside AddSC_boss_emalon() — loader decl 140 /
-- call 335, the "// Vault of Archavon" block in
-- AddNorthrendScripts(), northrend_script_loader.cpp).
-- Entries: 33993 Emalon the Storm Watcher (vault_of_archavon.h
-- NPC_EMALON, line 39; DATA_EMALON boss slot); 33998 Tempest Minion
-- (boss_emalon.cpp enum Npcs :54-57 — header carries no minion id).
-- The stale "no-timer-bridge" doctrine is superseded by the
-- CreateLuaEvent bridge (engine/scripting/runtime.go:798 — the
-- archavon / maiden_of_virtue precedent): module-local GUID-keyed
-- timers table, wipe on events 2 (leave combat), 4 (death), 23
-- (evade).
-- creature:CastSpell(nil, spell) = DoCastVictim; creature:CastSpell
-- (creature, spell) = DoCastSelf / DoCast(me); creature:CastSpell
-- (target, spell[, triggered]) = DoCast(target, spell[, triggered])
-- (the 3rd CastSpell arg is the C++ DoCast 3rd arg; 2-arg / 1-arg
-- overloads = non-triggered). All delays below are fixed ms (C++ uses
-- plain 5s/25s/40s/6min literals), so plain ms delays carry the
-- C++-exact timing — no {minMs, maxMs} randtime table.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_emalon.cpp):
-- boss_emalonAI::JustEngagedWith (:102-120) — the four ScheduleEvent
-- calls seed the combat timers below; the summons-list
-- minion-AttackStart leg and the BossAI::JustEngagedWith(who)
-- passthrough are unmodeled.
-- EVENT_CHAIN_LIGHTNING (:136-139) — SelectTarget(Random, 0) (:137,
-- no distance filter) -> DoCast(target, SPELL_CHAIN_LIGHTNING
-- 64213) (:138, 2-arg = non-triggered) -> CastSpell(target, 64213);
-- re-arm unconditional (:139), 25s. Pool: threat list, any unit,
-- alive-filtered (C++ SelectTarget skips dead/offline refs; Go
-- SortedEntries returns dead too — the anomalus / telestra class).
-- EVENT_LIGHTNING_NOVA (:141-143) — DoCastAOE(SPELL_LIGHTNING_NOVA
-- 64216) (:142) = self-cast per the in-tree DoCastAOE convention
-- (boss_ai.go) -> CastSpell(creature, 64216); re-arm 40s (:143).
-- EVENT_BERSERK (:158-160) — DoCast(me, SPELL_BERSERK 26662) (:159,
-- 2-arg = non-triggered) -> CastSpell(creature, 26662); Talk
-- (EMOTE_BERSERK 2) (:160) -> creature:Talk(2); cast-before-Talk kept;
-- 6min one-shot, no re-arm (:116).
-- npc_tempest_minionAI::JustEngagedWith (:219-229) — DoZoneInCombat()
-- (:221, head of the arm) PORTED in the event-1 handler below (live
-- bridge engine/world/lua_creature_events.go, 2026-10-23); EVENT_SHOCK
-- scheduled at 20s (:222); the instance->GetGuidData(DATA_EMALON)
-- pEmalon AttackStart leg is unmodeled.
-- EVENT_SHOCK (:265-268) — DoCastVictim(SPELL_SHOCK 64363) (:267,
-- 1-arg = non-triggered) -> CastSpell(nil, 64363); re-arm 20s
-- (:268).
-- EVENT_OVERCHARGE (:145-157) — PORTED (overchargeTick, 45s seed in
-- JustEngagedWith :115): random alive minion via GetCreaturesInGrid
-- (33998, 100y) + IsDead filter (C++ pick order unspecified);
-- minion->CastSpell(me, SPELL_OVERCHARGED 64217, true) (:151) ->
-- minion:CastSpell(creature, 64217, true) — C++-exact INCLUDING the
-- quirk that :151 targets `me` (Emalon) rather than the minion (see
-- DOCUMENTED-ONLY below); minion->SetFullHealth() (:152) ->
-- minion:SetHealth(minion:GetMaxHealth()); Talk(EMOTE_OVERCHARGE 0)
-- (:153) -> creature:Talk(0); re-arm 45s only inside the IsAlive gate
-- (:154), C++-exact.
-- DOCUMENTED DEVIATIONS: none on the ported arms — all five boss
-- timers and the minion shock timer carry C++-exact timing and
-- targeting. The UpdateAI HasUnitState(UNIT_STATE_CASTING) gates
-- (:129, :166) and DoMeleeAttackIfReady() (:170) have no bridge —
-- casts fire unconditionally on their timers (the hodir
-- UNIT_STATE_CASTING precedent). Minion pick: C++ draws from the
-- BossAI summons list (dead minions included, then IsAlive-gated);
-- Lua draws from the live grid list — same outcome when all four
-- minions are alive, benign delta otherwise.
-- DOCUMENTED-ONLY (in this header; entries 33993 / 33998):
-- boss_emalonAI::Reset (:85-91) — four SummonCreature
-- (NPC_TEMPEST_MINION, TEMPSUMMON_CORPSE_DESPAWN) at TempestMinions[]
-- positions PORTED this run (emalonReset, event 23 — the old "no
-- SummonCreature bridge" claim is re-keyed): summonType 4 /
-- despawnMs 0; documented delta: the corpse-despawn leg has no bridge
-- — minion corpses stay on the normal corpse sweep.
-- boss_emalonAI::JustSummoned (:93-100) — minion AttackStart leg: no
-- bridge.
-- npc_tempest_minionAI::JustDied (:207-217) — PORTED (minionJustDied,
-- event 4): the instance->GetGuidData(DATA_EMALON) leg is twinned by an
-- alive-entry grid lookup (overchargeTick idiom); Emalon summons the
-- replacement (all-zero coords -> 2*combatReach close-point fallback,
-- type 4) and the dying minion talks EMOTE_MINION_RESPAWN (1).
-- npc_tempest_minionAI::UpdateAI overcharge self-stack loop
-- (:242-263) — STAYS OUT (not for the old "no stack-count bridge"
-- reason: creature:GetStackAmount is bridged since this run,
-- engine/world/lua_creature_events.go). The seeding arm :151 casts
-- SPELL_OVERCHARGED on `me` (Emalon), never on the minion, so the
-- minion's own GetAura(SPELL_OVERCHARGED) can never be non-nil in
-- this tree — the :242-263 loop is inert in C++ too. Porting it
-- would add dead code; the quirk is kept 1:1 in overchargeTick.
-- DoZoneInCombat() (:221): PORTED (creature:DoZoneInCombat at the head
-- of minionEnterCombat — live bridge engine/world/lua_creature_events.go,
-- 2026-10-23).

local ENTRY_EMALON = 33993
local ENTRY_TEMPEST_MINION = 33998

local EMOTE_BERSERK = 2
local EMOTE_OVERCHARGE = 0
local EMOTE_MINION_RESPAWN = 1

local SPELL_CHAIN_LIGHTNING = 64213
local SPELL_LIGHTNING_NOVA = 64216
local SPELL_BERSERK = 26662
local SPELL_OVERCHARGED = 64217

local SPELL_SHOCK = 64363

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

-- C++ SelectTarget(SelectTargetMethod::Random, 0) — the bridged,
-- threat-ordered GetAITargets list (lua_creature_events.go:345);
-- playerOnly defaults FALSE (UnitAI.h:170), so the tempest-minion adds
-- on the threat list are eligible — no Player filter (the midnight
-- randomTargetAnywhere precedent). No distance filter.
local function randomThreatTarget(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:IsAlive() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_CHAIN_LIGHTNING (:136-139): DoCast(target,
-- SPELL_CHAIN_LIGHTNING), 2-arg = non-triggered; re-arm
-- unconditional, 25s.
local function chainLightningTick(creature, guid)
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_CHAIN_LIGHTNING)
    end
    schedule(guid, "chainlightning", 25000, function()
        chainLightningTick(creature, guid)
    end)
end

-- C++ EVENT_LIGHTNING_NOVA (:141-143): DoCastAOE(SPELL_LIGHTNING_NOVA)
-- = self-cast per the in-tree DoCastAOE convention; re-arm 40s.
local function lightningNovaTick(creature, guid)
    creature:CastSpell(creature, SPELL_LIGHTNING_NOVA)
    schedule(guid, "lightningnova", 40000, function()
        lightningNovaTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK (:158-160): DoCast(me, SPELL_BERSERK), 2-arg =
-- non-triggered; Talk(EMOTE_BERSERK); cast-before-Talk kept; 6min
-- one-shot, no re-arm.
local function emalonBerserkTick(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK)
    creature:Talk(EMOTE_BERSERK)
end

-- C++ TempestMinions[] (boss_emalon.cpp:63-69): the four Reset summon
-- positions.
local TEMPEST_MINION_SPAWNS = {
    { -203.980103, -281.287720, 91.650223, 1.598807 },
    { -233.489410, -281.139282, 91.652412, 1.598807 },
    { -233.267578, -297.104645, 91.681915, 1.598807 },
    { -203.842529, -297.097015, 91.745163, 1.598807 },
}

-- C++ boss_emalonAI::Reset (boss_emalon.cpp:85-91): _Reset() (timer
-- clear) then four SummonCreature(NPC_TEMPEST_MINION 33998,
-- TempestMinions[i], TEMPSUMMON_CORPSE_DESPAWN (4)) — the C++ call has
-- no despawn-time arg, so despawnMs 0. Documented delta: the
-- corpse-despawn leg has no bridge — minion corpses stay on the normal
-- corpse sweep. The JustSummoned AttackStart leg has no bridge
-- (AttackStart is unbridged); at Reset the C++ AttackStart is a
-- nullptr-guarded no-op (no victim), so nothing is lost.
local function emalonReset(event, creature)
    cancelTimers(creature:GetGUID())
    for _, p in ipairs(TEMPEST_MINION_SPAWNS) do
        creature:SummonCreature(ENTRY_TEMPEST_MINION, p[1], p[2], p[3], p[4], 4, 0)
    end
end

-- C++ EVENT_OVERCHARGE (boss_emalon.cpp:145-157): every 45s a random
-- minion is picked from the BossAI summons list and, if alive:
-- minion->CastSpell(me, SPELL_OVERCHARGED 64217, true) (:151),
-- minion->SetFullHealth() (:152), Talk(EMOTE_OVERCHARGE 0) (:153);
-- re-arm 45s (:154) sits INSIDE the IsAlive gate, so a dead pick (or an
-- empty list) stops the event. C++ QUIRK (kept 1:1): :151 targets `me`
-- (Emalon), not the minion — against the design (the buff belongs on
-- the minion) and against the minion's own :242-263 self-stack loop,
-- which reads the MINION's aura and is therefore inert in this tree
-- (documented stay-out below). Minion picked via GetCreaturesInGrid
-- (33998) + alive filter; C++ pick order is unspecified (benign).
-- SetFullHealth() = SetHealth(GetMaxHealth()) (both bridged).
local function overchargeTick(creature, guid)
    local minions = creature:GetCreaturesInGrid(ENTRY_TEMPEST_MINION, 100)
    if #minions > 0 then
        local minion = minions[math.random(#minions)]
        if not minion:IsDead() then
            minion:CastSpell(creature, SPELL_OVERCHARGED, true)
            minion:SetHealth(minion:GetMaxHealth())
            creature:Talk(EMOTE_OVERCHARGE)
            schedule(guid, "overcharge", 45000, function()
                overchargeTick(creature, guid)
            end)
        end
    end
end

-- C++ boss_emalonAI::JustEngagedWith (:102-120): the ScheduleEvent
-- calls seed the combat timers below (5s / 40s / 6min / 45s).
local function emalonEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "chainlightning", 5000, function()
        chainLightningTick(creature, guid)
    end)
    schedule(guid, "lightningnova", 40000, function()
        lightningNovaTick(creature, guid)
    end)
    schedule(guid, "berserk", 360000, function()
        emalonBerserkTick(creature, guid)
    end)
    schedule(guid, "overcharge", 45000, function()
        overchargeTick(creature, guid)
    end)
end

-- Shared timer wipe for events 2/23/4 (the svala / yogg_saron
-- convention).
local function emalonCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_EMALON, 1, emalonEnterCombat)
RegisterCreatureEvent(ENTRY_EMALON, 2, emalonCancelCombatTimers)
RegisterCreatureEvent(ENTRY_EMALON, 4, emalonCancelCombatTimers)
RegisterCreatureEvent(ENTRY_EMALON, 23, emalonReset)

-- C++ EVENT_SHOCK (:265-268): DoCastVictim(SPELL_SHOCK), 1-arg =
-- non-triggered; re-arm 20s.
local function shockTick(creature, guid)
    creature:CastSpell(nil, SPELL_SHOCK)
    schedule(guid, "shock", 20000, function()
        shockTick(creature, guid)
    end)
end

-- C++ npc_tempest_minionAI::JustEngagedWith (:219-229): DoZoneInCombat
-- (:221, head of the arm) PORTED (live bridge
-- engine/world/lua_creature_events.go, 2026-10-23); EVENT_SHOCK
-- scheduled at 20s (:222). The instance leg is unmodeled (documented
-- in the header).
local function minionEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:DoZoneInCombat() -- C++ :221
    schedule(guid, "shock", 20000, function()
        shockTick(creature, guid)
    end)
end

local function minionCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_tempest_minionAI::JustDied (boss_emalon.cpp:207-217): if
-- Emalon is alive, EMALON summons the replacement minion
-- (SummonCreature(NPC_TEMPEST_MINION, 0,0,0,0,
-- TEMPSUMMON_CORPSE_DESPAWN (4)) — all-zero coords hit the
-- 2*combatReach close-point fallback of the summoner, i.e. Emalon)
-- and the dying minion talks EMOTE_MINION_RESPAWN (1). The
-- instance-GUID leg is twinned by an alive-entry grid lookup (the
-- overchargeTick idiom).
local function minionJustDied(event, creature)
    local bosses = creature:GetCreaturesInGrid(ENTRY_EMALON, 100)
    for _, emalon in ipairs(bosses) do
        if not emalon:IsDead() then
            emalon:SummonCreature(ENTRY_TEMPEST_MINION, 0, 0, 0, 0, 4)
            creature:Talk(EMOTE_MINION_RESPAWN)
            return
        end
    end
end

RegisterCreatureEvent(ENTRY_TEMPEST_MINION, 1, minionEnterCombat)
RegisterCreatureEvent(ENTRY_TEMPEST_MINION, 2, minionCancelCombatTimers)
RegisterCreatureEvent(ENTRY_TEMPEST_MINION, 4, minionCancelCombatTimers)
RegisterCreatureEvent(ENTRY_TEMPEST_MINION, 23, minionCancelCombatTimers)
RegisterCreatureEvent(ENTRY_TEMPEST_MINION, 4, minionJustDied)
