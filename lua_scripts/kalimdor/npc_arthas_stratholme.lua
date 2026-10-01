-- Culling of Stratholme: Arthas (npc_arthas_stratholme) — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/CullingOfStratholme/
-- npc_arthas.cpp (1676 lines; class npc_arthas_stratholme : public
-- CreatureScript { npc_arthas_stratholmeAI : public ScriptedAI } +
-- npc_stratholme_rp_dummy (NullCreatureAI, MovementInform reporting) +
-- spell_stratholme_crusader_strike (SpellScript); all GetAI via
-- GetCullingOfStratholmeAI (CoSScriptName "instance_culling_of_
-- stratholme" gate); AddSC_npc_arthas_stratholme at end registers all
-- three; kalimdor loader decl 48 / call 161 per
-- kalimdor_script_loader.cpp — the second "// CoT Culling Of
-- Stratholme" loader group, right after AddSC_boss_epoch()).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole source
-- of "npc_arthas_stratholme" (loader decl/call lines only otherwise).
-- Entry: culling_of_stratholme.h:161 names NPC_ARTHAS = 26499 AND
-- instance_culling_of_stratholme.cpp:529 has
-- instance->SummonCreature(NPC_ARTHAS, GetArthasSnapbackFor(...)) with
-- :619 casing NPC_ARTHAS in OnCreatureCreate — the name-to-entry tie is
-- C++-verified at summon strength; creature_template ScriptName binding
-- stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady. The engine
-- Talk bridge takes only the text id (no target), so the C++ talk-target
-- legs (who / _eventStarterGuid) collapse to the bare line.
-- Ported arms (npc_arthas_stratholme's self-contained in-combat legs —
-- legoso combat-rotation-only precedent):
-- - Engage (C++ JustEngagedWith): no Talk in C++; the _progressRP = false,
--   SetHomePosition, and SplineChainMovementGenerator::GetResumeInfo legs
--   are RP-state only. The engage starts the combat pump below.
-- - UpdateAICombat: C++ gates the whole rotation on
--   !me->HasReactState(REACT_PASSIVE) — the react-state leg has no
--   bridge (documented below); the ported arms fire whenever Arthas is
--   actually engaged, which in C++ only happens in the non-passive phases.
--   - Holy Light 52444: self-cast (C++ DoCastSelf) whenever
--     HealthBelowPct(40), checked every AI tick with no cooldown — modeled
--     as a 1s per-GUID pump (sironas convention); the 1s granularity is an
--     approximation, like old_hillsbrad's LowHp leg.
--   - Exorcism 52445: _exorcismCooldown urand(7,14)s in the ctor; on
--     expiry SelectTarget(Random, 0) with no range cap -> DoCast, then
--     re-arm urand(7,14)s. The C++ "still casting -> _exorcismCooldown =
--     0 (retry next tick)" leg has no casting-state bridge (the aeonus
--     casting-gate case) — the pump retries next tick anyway, which
--     matches. The no-range-cap SelectTarget resolves to an unbounded
--     random alive player (nefarian randomAlivePlayer convention, deja);
--     nil target -> no cast, retry next pump (C++ leaves the cooldown
--     unset, retrying next tick).
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds): C++ gates on
--   who->GetEntry() == NPC_RISEN_ZOMBIE (27737) before Talk
--   LINE_SLAY_ZOMBIE (39), with who as the talk target.
-- - Death (C++ JustDied): instance->SetData(DATA_ARTHAS_DIED, 1) +
--   DespawnOrUnsummon(5s) — instance bookkeeping blocked (standing); the
--   Go leg is cancel only.
-- Unmodeled (documented-only, no bridges):
-- - The entire RP escort machine: AdvanceToState (snapback positions +
--   SetReactState / SetImmuneToAll / gossip-flag legs for the 17
--   COSProgressStates), ScheduleActionOOC / DoAction (RP3_ACTION_AFTER_*
--   + RP5_ACTION_AFTER_MALGANIS), SetGUID's -ACTION_START_RP_EVENT2/3/
--   4_1/4_2/5 legs, the MovementInform point-id -> event cascade (RP1-5
--   waypoint ids), EngageInfinites / MoveInfiniteOnSpawn, all
--   MoveAlongSplineChain / LaunchMoveSpline / NearTeleportTo legs, the
--   ~130 RP event handlers (Talks, Talk-lines 0-40, emotes, casts,
--   summons, FindNearestCreature/SetFacingTo facing legs, GO state
--   changes, instance SetData/GetData legs), and EnterEvadeMode (trace
--   only). No spline-chain / summon-with-position / instance-data /
--   nearby-creature / gameobject bridges exist.
-- - JustAppeared: DoCastSelf(SPELL_DEVOTION_AURA 52442) — no
--   spawn/appear bridge in the Lua API.
-- - CanAIAttack: REACT_AGGRESSIVE chase capped at 30.0f 2D from the
--   home/relative position — no chase-range bridge.
-- - npc_stratholme_rp_dummy (NullCreatureAI whose MovementInform forwards
--   spline-chain ids to the summoner's AI): zero bridgeable arms —
--   documented, not registered.
-- - spell_stratholme_crusader_strike (SpellScript: EFFECT_0
--   SPELL_EFFECT_DUMMY -> if the hit unit is NPC_CITIZEN 28167 or
--   NPC_RESIDENT 28169, Unit::Kill): SpellScript check handlers are not
--   modeled (standing blocked queue) — documented, not registered.
-- - All RP1-5 Talk lines (0-40) fire only inside the unbridgeable RP
--   machine, so no bridgeable firing context exists for them; the unused
--   SPELL_DEVOTION_AURA self-buff outside JustAppeared and the unused
--   SPELL_SHADOWSTEP_VISUAL / SPELL_TRANSFORM_VISUAL consumer legs have
--   no bridgeable context either.

local ENTRY = 26499

local LINE_SLAY_ZOMBIE = 39

local SPELL_HOLY_LIGHT = 52444
local SPELL_EXORCISM   = 52445

local NPC_RISEN_ZOMBIE = 27737

local pumps = {}

local function cancelPump(guid)
    local id = pumps[guid]
    if id then
        RemoveEventById(id)
        pumps[guid] = nil
    end
end

-- nefarian convention: unbounded random alive player in the instance.
local function randomAlivePlayer(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ UpdateAICombat legs: HealthBelowPct(40) -> DoCastSelf(52444) every
-- AI tick; _exorcismCooldown (init urand(7,14)s from the AI ctor) on
-- expiry -> DoCast(SelectTarget(Random, 0), 52445), then re-arm
-- urand(7,14)s. 1s per-GUID pump (sironas convention); C++'s per-tick
-- checks make this an approximation (like old_hillsbrad). The C++
-- "still casting -> retry next tick" leg has no casting-state bridge
-- (aeonus case) — the pump's next tick is the retry.
local function startCombatPump(creature, guid)
    local exorcism = math.random(7, 14)
    pumps[guid] = CreateLuaEvent(function()
        if creature:GetHealthPct() < 40 then
            creature:CastSpell(creature, SPELL_HOLY_LIGHT)
        end
        exorcism = exorcism - 1
        if exorcism <= 0 then
            local target = randomAlivePlayer(creature)
            if target then
                creature:CastSpell(target, SPELL_EXORCISM)
                exorcism = math.random(7, 14)
            else
                exorcism = 0 -- C++ leaves the cooldown unset on nil target; retry next tick
            end
        end
    end, 1000, 0)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelPump(guid)
    startCombatPump(creature, guid)
end

local function onLeaveCombat(event, creature)
    cancelPump(creature:GetGUID())
end

local function onReset(event, creature)
    cancelPump(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelPump(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    if victim:GetEntry() == NPC_RISEN_ZOMBIE then
        creature:Talk(LINE_SLAY_ZOMBIE)
    end
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
