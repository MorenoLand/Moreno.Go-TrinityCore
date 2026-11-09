-- Battle for Mount Hyjal: Anetheron — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/BattleForMountHyjal/
-- boss_anetheron.cpp (310 lines; boss_anetheronAI : public hyjal_trashAI :
-- public EscortAI; npc_towering_infernal : public ScriptedAI;
-- spell_anetheron_vampiric_aura SpellScriptLoader; AddSC_boss_anetheron at
-- end registers all three; kalimdor loader decl 31 / call 144 per
-- kalimdor_script_loader.cpp).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole source
-- of "boss_anetheron", "npc_towering_infernal" and
-- "spell_anetheron_vampiric_aura" (loader lines only otherwise).
-- Entry: hyjal.h HYCreaturesIds names ANETHERON = 17808 under the "Bosses
-- summoned after every 8 waves" comment AND instance_hyjal.cpp:119 cases
-- it in OnCreatureCreate (Anetheron GUID capture, :155 the DATA_ANETHERON
-- GetGuidData leg) — the name-to-entry tie is C++-verified (ramstein
-- strength); the creature_template ScriptName binding stays DB-side.
-- GetAI uses GetHyjalAI<boss_anetheronAI>, same as jaina/thrall.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs; the IsEvent escort
-- machine needs an EscortAI + instance bridge and has none — see below):
-- - Constructor Initialize() arms the four timers at fixed values, so
--   engage arms each timer at its C++ init cooldown; expiry re-arms at the
--   C++ repeat ranges (jeklik convention):
--   - Carrion Swarm 31306 on a random alive player within 100 (C++
--     SelectTargetMethod::Random, 0, 100, true — threat-list player pool
--     (ras_frostwhisper precedent)) init 45s -> {45s,60s}, non-triggered.
--     C++ DoCast accepts a nil target (no-op cast) and STILL talks and
--     re-arms — modeled exactly: cast only if the target is alive,
--     Talk SAY_SWARM (2) unconditionally, then re-arm.
--   - Sleep 31298: 3 iterations, each picking a random alive player
--     within 100 (C++ re-selects per iteration — with replacement), the
--     player casts 31298 on itself triggered (C++ target->CastSpell(target,
--     SPELL_SLEEP, true) — aku_mai triggered-form convention), init 60s
--     -> 60s, Talk SAY_SLEEP (3) unconditionally on expiry.
--   - Vampiric Aura 38196 self-cast, triggered, init 5s -> {10s,20s}.
--   - Inferno 31299 on a random alive player within 100 (nil target is a
--     no-op cast, Talk + re-arm still fire — same C++ leg as Swarm),
--     non-triggered, init 45s -> 45s, Talk SAY_INFERNO (4).
-- - Engage (C++ JustEngagedWith): SetBossState(DATA_ANETHERON 1,
--   IN_PROGRESS) before Talk SAY_ONAGGRO (5) (boss_anetheron.cpp:90-94).
-- - KilledUnit: Talk SAY_ONSLAY (1), TYPEID_PLAYER gate in C++
--   (terestian_illhoof victim:GetObjectType() == "Player" convention) —
--   event 3.
-- - Death (C++ JustDied): SetBossState(DATA_ANETHERON 1, DONE) after
--   hyjal_trashAI::JustDied, before Talk SAY_ONDEATH (0) (:114-118).
-- Unmodeled (documented-only, no bridges):
-- - The IsEvent escort machine: the go-flagged first-UpdateAI block adds
--   8 C++-verbatim waypoints (4896.08,-1576.35,1333.65 /
--   4898.68,-1615.02,1329.48 / 4907.12,-1667.08,1321.00 /
--   4963.18,-1699.35,1340.51 / 4989.16,-1716.67,1335.74 /
--   5026.27,-1736.89,1323.02 / 5037.77,-1770.56,1324.36 /
--   5067.23,-1789.95,1321.17), Start(false, true),
--   SetDespawnAtEnd(false); WaypointReached(7) AddThreat(0) on the
--   instance DATA_JAINAPROUDMOORE GUID — EscortAI movement plus
--   instance-script model, both blocked (standing; the whole hyjal_trashAI
--   escort/wave machine is blocked the same way, see hyjal_trash.lua).
-- - SetBossState legs PORTED this tick (bridge live
--   lua_creature_events.go:5917; the DATA_*EVENT slot 2 is the C++
--   encounter-array id, the DATA id 1 is the Go boss-state key —
--   ruby-sanctum convention): Reset writes NOT_STARTED (:84-87,
--   alive-gated, NO != DONE gate — these AIs are hyjal_trashAI /
--   EscortAI, not BossAI, so no _Reset gate applies; EscortAI::
--   EnterEvadeMode calls Reset(), ScriptedEscortAI.cpp:114, so evade
--   writes NOT_STARTED too, never FAIL — no ev2/ev24 leg). The C++
--   `if (IsEvent)` gate collapses: the entry only ever fights as the
--   event boss (summoned by hyjalAI::SummonCreature with IsEvent=true,
--   hyjalAI.cpp:520-523), same as the combat arms ported unconditionally.
--   The constructor Initialize() timer reset collapses into the engage
--   re-arm (hyjal.lua convention).
-- - hyjal_trashAI::JustDied: instance->SetData(DATA_TRASH, 0) wave signal
--   plus the MINRAIDDAMAGE lootable-flag gate — blocked (standing).
-- - npc_towering_infernal: IS registered in C++ (new
-- npc_towering_infernal() in AddSC_boss_anetheron) but unportable with zero
-- C++ entry evidence (no hyjal.h constant, zero "towering" hits outside
-- boss_anetheron.cpp + loader): registering would invent an identifier
-- (gelihast precedent). Combat arms (fully bridgeable, awaiting entry
-- verification):
--   Reset self-casts SPELL_INFERNO_EFFECT 31302 (non-triggered,
--   C++-exact); Immolation 31303 self init 5s -> 5s (non-triggered);
--   MoveInLineOfSight AttackStart within 50 while not in combat (no
--   LoS-aggro bridge, standing); CheckTimer 5s: DespawnOrUnsummon when the
--   instance's DATA_ANETHERON GUID is missing/dead (instance GUID bridge
--   blocked, standing).
-- - spell_anetheron_vampiric_aura AuraScript: OnEffectProc HandleProc
--   (PreventDefaultAction; heal = damage * 3 via SPELLVALUE_BASE_POINT0
--   into SPELL_VAMPIRIC_AURA_HEAL 31285) — PORTED 2026-11-08 (tick 431,
--   below) via the creature-side AuraEventOnProc bridge (event 6,
--   fireCreatureAuraProcLoop, boss_ai.go).

local ENTRY = 17808

local DATA_ANETHERON = 1 -- hyjal.h:29

-- EncounterState (InstanceScript.h:68-76): NOT_STARTED 0, IN_PROGRESS 1, DONE 3.
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

local SAY_ONDEATH = 0
local SAY_ONSLAY  = 1
local SAY_SWARM   = 2
local SAY_SLEEP   = 3
local SAY_INFERNO = 4
local SAY_ONAGGRO = 5

-- C++-verbatim timer table: spell, lo/hi engage-arm range,
-- rlo/rhi repeat range, target "self"/"random"/"sleep",
-- triggered flag, say (Talk fired unconditionally on timer
-- expiry — C++ talks even when the SelectTarget is nil),
-- randomDist for the random-player arms.
local SPELLS = {
    { spell = 31306, lo = 45000, hi = 45000, rlo = 45000, rhi = 60000,
      target = "random", randomDist = 100, say = SAY_SWARM },
    { spell = 31298, lo = 60000, hi = 60000, rlo = 60000, rhi = 60000,
      target = "sleep", randomDist = 100, say = SAY_SLEEP },
    { spell = 38196, lo = 5000, hi = 5000, rlo = 10000, rhi = 20000,
      target = "self", triggered = true },
    { spell = 31299, lo = 45000, hi = 45000, rlo = 45000, rhi = 45000,
      target = "random", randomDist = 100, say = SAY_INFERNO },
}

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

-- C++ SelectTarget(Random, 0, maxDist, true): random threat-list
-- player within maxDist yards, victim included (UnitAI.h:
-- SelectTarget draws from the threat list; offset 0, no skip;
-- ras_frostwhisper precedent).
local function randomPlayerInRange(creature, maxDist)
    local found = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < maxDist then
            found[#found + 1] = t
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

local function onSpell(creature, guid, idx)
    local spec = SPELLS[idx]
    if spec.target == "self" then
        creature:CastSpell(creature, spec.spell, spec.triggered)
    elseif spec.target == "sleep" then
        for i = 1, 3 do
            local target = randomPlayerInRange(creature, spec.randomDist)
            if target and not target:IsDead() then
                -- C++ boss_anetheron.cpp:160: target->CastSpell(target,
                -- SPELL_SLEEP, true) — ported as the target self-cast
                -- via player:CastSpell (bridged 2026-10-24, lua_player.go).
                target:CastSpell(target, spec.spell, true)
            end
        end
    else
        local target = randomPlayerInRange(creature, spec.randomDist)
        if target and not target:IsDead() then
            creature:CastSpell(target, spec.spell)
        end
    end
    if spec.say then
        creature:Talk(spec.say)
    end
    schedule(guid, "spell" .. idx,
        math.random(spec.rlo, spec.rhi), function()
            onSpell(creature, guid, idx)
        end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ JustEngagedWith (:90-94): SetData(DATA_ANETHERONEVENT,
    -- IN_PROGRESS) first, then Talk.
    creature:SetBossState(DATA_ANETHERON, BOSS_STATE_IN_PROGRESS)
    creature:Talk(SAY_ONAGGRO)
    for i, spec in ipairs(SPELLS) do
        local id = i
        schedule(guid, "spell" .. i, math.random(spec.lo, spec.hi),
            function()
                onSpell(creature, guid, id)
            end)
    end
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ Reset (:84-87): Initialize() then SetData(NOT_STARTED).
    -- Alive-gated: Go fires ev23 on the death path before ev4
    -- (moroes/algalon precedent) and C++ Reset never runs on death.
    -- NO != DONE gate (see header): this AI is hyjal_trashAI /
    -- EscortAI, not BossAI.
    if creature:IsAlive() then
        creature:SetBossState(DATA_ANETHERON, BOSS_STATE_NOT_STARTED)
    end
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    -- C++ JustDied (:114-118): hyjal_trashAI::JustDied first, then
    -- SetData(DONE), then Talk.
    creature:SetBossState(DATA_ANETHERON, BOSS_STATE_DONE)
    creature:Talk(SAY_ONDEATH)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_ONSLAY)
    end
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)

-- C++ spell_anetheron_vampiric_aura::HandleProc
-- (boss_anetheron.cpp:280-292): OnEffectProc, EFFECT_0 SPELL_AURA_DUMMY.
-- PreventDefaultAction (the handler returns true, the AuraEventOnProc
-- veto) + the damage>0 gate, then the actor (the damage dealer, i.e.
-- Anetheron on the done side) self-casts SPELL_VAMPIRIC_AURA_HEAL
-- 31285 with SPELLVALUE_BASE_POINT0 = damage * 3, via the aurEff
-- overload (triggered cast). Rides the Go AuraEventOnProc bridge
-- (event 6, fireCreatureAuraProcLoop, boss_ai.go); the 38196
-- self-cast is ported above (5s -> {10s,20s} timer), so the leg can
-- fire live whenever Anetheron deals damage. PORTED 2026-11-08
-- (tick 431). DORMANT until the DBC proc entry for 38196 ships
-- (serverSpellProcEntryFor needs Spell.dbc ProcTypeMask).
local SPELL_VAMPIRIC_AURA = 38196 -- boss_anetheron.cpp:29
local SPELL_VAMPIRIC_AURA_HEAL = 31285 -- boss_anetheron.cpp:30

local function vampiricAuraProc(event, aura, target, actor, procTarget, damage)
    if damage == 0 or actor == nil then
        return true
    end
    actor:CastCustomSpell(actor, SPELL_VAMPIRIC_AURA_HEAL, true, damage * 3)
    return true
end

RegisterAuraEvent(SPELL_VAMPIRIC_AURA, 6, vampiricAuraProc)
