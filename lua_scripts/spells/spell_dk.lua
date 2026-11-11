-- Death Knight spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_dk.cpp.
--
-- PORTED (tick 294, RegisterAuraEvent):
--   spell_dk_presence (spell_dk.cpp:1859-1989): the three presence auras
--   48266 (Blood), 48263 (Frost), 48265 (Unholy). One mode-2 handler per
--   ID runs all three legs, branching on aura:GetAuraId(), because C++
--   registers all three AfterEffectApply handlers on each presence
--   (AURA_EFFECT_HANDLE_REAL, EFFECT_0, SPELL_AURA_ANY).
--   Blood leg: id 48266 casts 63611 triggered; else the Improved Blood
--   Presence effect (GetAuraEffectOfRankedSpell(50365, 0)) casts 63611
--   with BP1 = that amount, unless 63611 is already present.
--   Frost leg: id 48263 casts 61261 triggered; else the Improved Frost
--   Presence effect (50384, 0) casts 61261 with BP0 = that amount,
--   unless 61261 is already present.
--   Unholy leg: id 48265 casts 49772 triggered; the Improved Unholy
--   Presence effect (50391, 0), when present, casts 63622 with all
--   three BPs = the EFFECT_1 CalcValue of spell 50391 (DBC read, 0
--   skips the cast), or casts 49772 with BP0 = that amount when a
--   different presence is up and 49772 is absent.
--   Mode 3 (all three IDs): unconditionally removes 63611, 63622,
--   61261 and 49772.
--   spell_dk_improved_blood_presence (spell_dk.cpp:1442-1493): aura
--   50365 (rank 1 only; ranks 2-3 IDs are not in the file, never
--   guessed). Mode 2: while Frost (48263) or Unholy (48265) presence
--   is up and 63611 is absent, casts 63611 with BP1 = this aura's
--   EFFECT_0 amount. Mode 3: removes 63611 unless Blood presence
--   (48266) is up.
--   spell_dk_improved_frost_presence (spell_dk.cpp:1541-1592): aura
--   50384 (rank 1 only). Mode 2: while Blood (48266) or Unholy
--   (48265) presence is up and 61261 is absent, casts 61261 with BP0
--   = this aura's EFFECT_0 amount. Mode 3: removes 61261 unless
--   Frost presence (48263) is up.
--   spell_dk_improved_unholy_presence (spell_dk.cpp:1593-1658):
--   aura 50391 (rank 1 only). Mode 2 leg A: while Unholy presence
--   (48265) is up and 63622 is absent, casts 63622 with all three
--   BPs = the EFFECT_1 CalcValue of spell 50391 (DBC read, 0 skips
--   the cast). Leg B: while Blood (48266) or Frost (48263) presence
--   is up and 49772 is absent, casts 49772 with BP0 = this aura's
--   EFFECT_0 amount. Mode 3: removes 63622 unconditionally and
--   removes 49772 unless Unholy presence (48265) is up.
--   CastCustomSpell nil BP args skip that effect (luaCastCustomBasePoints),
--   so the BP1-only 63611 cast passes nil for BP0.
--   Mode 2 fires on fresh apply only; the C++ legs are all
--   AURA_EFFECT_HANDLE_REAL, so this matches C++ (no reapply leg).
-- STAY-OUT (tick 294): every other spell_dk class — no proc hook
--   (OnEffectProc/CheckProc), absorb/CalcAmount/ManaShield hooks
--   absent, or SpellScript-only with no RegisterSpellScript in the
--   engine. spell_dk_death_and_decay ranks 1-2 (43265/49938) are
--   PORTED below (ev1, tick 547); ranks 3-4 (49939/49940) stay out —
--   consecutive IDs with no in-tree citation.
--   spell_dk_blood_tap needs the rune system; spell_dk_hysteria
--   needs DealDamage; spell_dk_raise_ally needs ghoul AI push/pop.
--   spell_dk_death_coil ranks 1-4 (47541/52375/59134/62900) stay out
--   of Lua — no ev5 player path, no IsFriendlyTo on players; the Go
--   finishSpellCast dummy leg is PORTED (spells.go deathCoilDummyEffect
--   + boss_ai.go creature twin: 47632 damage / 47633 heal, BP0 = effect
--   value [*1.5 heal] + sigil 64962 eff1).
--   spell_dk_death_rune (49208/49467/54639, tick 559) stays out — no
--   Lua rune bridges at all (GetBaseRune/GetRuneCooldown/
--   AddRuneByAuraEffect/RemoveRunesByAuraEffect, aura ResetPeriodic
--   all absent; Go-side dkRuneState exists, runes.go). spell_dk_
--   death_strike (49998, tick 562) PORTED in Go (spells.go
--   deathStrikeDummyEffect, this tick): disease count * max-health
--   pct (Effects[0].DamageMultiplier = ChainAmplitude) + Improved
--   Death Strike icon 2751 leg, self-cast 45470; the C++ cast is
--   non-triggered but rides the triggered funnel (no non-triggered
--   player cast helper — the 47496 ghoul precedent).
-- GO-PORTED (this tick, engine/world/spells.go — NOT Lua: the player
--   path has no OnEffectHitTarget site on the normal cast path, so a
--   RegisterSpellEvent ev5 handler would never fire on a client cast):
--   spell_dk_corpse_explosion (49158/51325-51328, spell_dk.cpp:647-745):
--   Go case in the finishSpellCast dummy leg (60587 precedent) —
--   EFFECT_0 takes the explicit target when it passes
--   CorpseExplosionCheck (dead corpse or own ghoul 26125, not
--   mechanical/elemental, display == native), EFFECT_1 explodes one
--   random valid corpse from the area list when no valid explicit
--   target exists; damage leg casts the EFFECT_1 CalcValue spell
--   with BP0 = EFFECT_0 value, then 43999 + 51270 triggered. Deltas:
--   empty-list FinishCast has no bridge (cast does nothing); the
--   living-ghoul leg casts 47496 triggered where C++ casts plain,
--   and its Suicide leg (43999 + 51270) ports as the ev5 handler
--   below (2026-11-10) — the Go detonate arm must not also cast them
--   for a living ghoul, or it would double-fire.
--   spell_dk_death_strike (49998, spell_dk.cpp:1126-1165, tick 562):
--   Go case in the finishSpellCast dummy leg — per hit target the
--   caster self-casts 45470 with BP0 = diseases-by-caster *
--   CountPctFromMaxHealth(Effects[0].DamageMultiplier), plus the
--   Improved Death Strike icon-2751 AddPct leg (aura spell EFFECT_2).
--   KNOWN DELTA: C++ casts the heal non-triggered; Go has no
--   non-triggered player cast helper, so it rides the triggered
--   funnel (47496 precedent). DORMANT until Spell.dbc ships the
--   49998 + 45470 records; other Death Strike ranks unconfirmed.
-- ALREADY PORTED (verified in-house this tick, NOT re-ported — the
--   2026-11-10 census rows for these two are stale, its matcher
--   misses Go cases in spells.go/procs.go):
--   spell_dk_dancing_rune_weapon (49028, spell_dk.cpp:747-807):
--   Go twin in procs.go (dbd71fab) — the proc loop finds the DRW
--   (entry 27893, owner == caster) by motion-map scan and mirrors
--   half the proc-event damage onto its victim with the damage log
--   + direct damage, using the proc spell's id/school from the Go
--   proc event (exactly the three things the Lua ev6 args lack:
--   no controlled-unit enumeration, no proc spell id/school, no
--   damage-log bridge — the Lua stay-out analysis still holds, the
--   Go twin is the only leg). DORMANT until Spell.dbc ships the
--   49028 proc entry.
--   spell_dk_death_gate (52751, spell_dk.cpp:909-948): pre-existing
--   Go case in handleEffectScriptEffect — the hit unit self-casts
--   the EFFECT_0 value (the teleport spell); CheckClass stays out
--   (no OnCheckCast bridge); the player leg rides the triggered
--   funnel where C++ casts plain (documented delta). DORMANT until
--   Spell.dbc ships 52751.
--   spell_dk_dancing_rune_weapon (49028, 2026-11-10): Lua STAY-OUT,
--   PORTED in Go (procs.go procAuraTriggerLoopFiltered, sibling tick):
--   the OnEffectProc arm mirrors half the triggering spell's damage
--   from the controlled rune-weapon guardian (entry 27893) via
--   SendSpellNonMeleeDamageLog + DealDamage. No Lua ev6 port: the proc
--   args carry no triggering spell id/school, Lua has no
--   controlled-unit enumeration, and the Lua DealDamage spell leg
--   deals 0 by design (lua_unit_combat.go). The Go twin scans the
--   motion map for entry 27893 owned by the caster and deals the
--   damage through the log + direct-damage helpers.
--   spell_dk_death_gate (52751, 2026-11-10): PORTED in Go, not Lua —
--   handleEffectScriptEffect case (spells.go): the hit unit self-casts
--   the effect-0 CalcValue non-triggered; a Lua ev5 port would never
--   fire (the player path has no OnEffectHitTarget site, 60533
--   precedent) and the OnCheckCast class gate stays out (no check-cast
--   bridge; DK-gated by learning).
--   spell_dk_blood_boil (48721): PORTED below (2026-11-10).
--   spell_dk_ghoul_explode (47496): PORTED below (2026-11-10) — the
--   Suicide leg as an ev5 handler; the HandleDamage leg is the Go
--   spellScriptOnHitDamage arm (spell_onhit.go).
--   spell_dk_advantage_t10_4p (70656, 2026-11-10): PORTED in Go
--   (procs.go advantageT10ProcAllowed): DoCheckProc gate only — DK
--   player actor with all runes on cooldown; the 70657 trigger is
--   DBC-driven. No Lua ev6 twin: no rune-cooldown bridge in Lua.
--   spell_dk_anti_magic_shell_raid (50462, 2026-11-10): PORTED in Go
--   (absorption.go scriptAbsorbHit): per-hit absorb is pct of damage,
--   pool unlimited (-1 in C++), never consumed. No Lua absorb hook.
--   spell_dk_anti_magic_zone (50461, 2026-11-10): PORTED in Go
--   (absorption.go scriptAbsorbHit + spells.go auraEffectParams):
--   per-hit absorb pct of damage, pool = 51052 talent eff0 + 2x AP.
--   spell_dk_blood_gorged (50453, 2026-11-10): PORTED in Go
--   (boss_ai.go creature proc loop): bloodworm casts 50454 on its
--   owner, BP0 = 150% of triggering damage; owner-missing gate is
--   the DoCheckProc leg. No Lua handler registered (GetOwner +
--   CastCustomSpell exist, so a future ev6 handler would veto first).

local SPELL_DK_BLOOD_PRESENCE = 48266
local SPELL_DK_FROST_PRESENCE = 48263
local SPELL_DK_UNHOLY_PRESENCE = 48265
local SPELL_DK_IMPROVED_BLOOD_PRESENCE_R1 = 50365
local SPELL_DK_IMPROVED_FROST_PRESENCE_R1 = 50384
local SPELL_DK_IMPROVED_UNHOLY_PRESENCE_R1 = 50391
local SPELL_DK_IMPROVED_BLOOD_PRESENCE_TRIGGERED = 63611
local SPELL_DK_FROST_PRESENCE_TRIGGERED = 61261
local SPELL_DK_IMPROVED_UNHOLY_PRESENCE_TRIGGERED = 63622
local SPELL_DK_UNHOLY_PRESENCE_TRIGGERED = 49772

local function presenceApply(event, aura, target)
    local id = aura:GetAuraId()
    if id == SPELL_DK_BLOOD_PRESENCE then
        target:CastSpell(target, SPELL_DK_IMPROVED_BLOOD_PRESENCE_TRIGGERED, true)
    else
        local imp = target:GetAuraEffectOfRankedSpell(SPELL_DK_IMPROVED_BLOOD_PRESENCE_R1, 0)
        if imp and not target:HasAura(SPELL_DK_IMPROVED_BLOOD_PRESENCE_TRIGGERED) then
            target:CastCustomSpell(target, SPELL_DK_IMPROVED_BLOOD_PRESENCE_TRIGGERED, true, nil, imp)
        end
    end
    if id == SPELL_DK_FROST_PRESENCE then
        target:CastSpell(target, SPELL_DK_FROST_PRESENCE_TRIGGERED, true)
    else
        local imp = target:GetAuraEffectOfRankedSpell(SPELL_DK_IMPROVED_FROST_PRESENCE_R1, 0)
        if imp and not target:HasAura(SPELL_DK_FROST_PRESENCE_TRIGGERED) then
            target:CastCustomSpell(target, SPELL_DK_FROST_PRESENCE_TRIGGERED, true, imp)
        end
    end
    if id == SPELL_DK_UNHOLY_PRESENCE then
        target:CastSpell(target, SPELL_DK_UNHOLY_PRESENCE_TRIGGERED, true)
    end
    local imp = target:GetAuraEffectOfRankedSpell(SPELL_DK_IMPROVED_UNHOLY_PRESENCE_R1, 0)
    if imp then
        if id == SPELL_DK_UNHOLY_PRESENCE then
            local bp = aura:GetSpellEffectCalcValue(SPELL_DK_IMPROVED_UNHOLY_PRESENCE_R1, 1)
            if bp ~= 0 then
                target:CastCustomSpell(target, SPELL_DK_IMPROVED_UNHOLY_PRESENCE_TRIGGERED, true, bp, bp, bp)
            end
        elseif not target:HasAura(SPELL_DK_UNHOLY_PRESENCE_TRIGGERED) then
            target:CastCustomSpell(target, SPELL_DK_UNHOLY_PRESENCE_TRIGGERED, true, imp)
        end
    end
end

local function presenceRemove(event, aura, target)
    target:RemoveAura(SPELL_DK_IMPROVED_BLOOD_PRESENCE_TRIGGERED)
    target:RemoveAura(SPELL_DK_IMPROVED_UNHOLY_PRESENCE_TRIGGERED)
    target:RemoveAura(SPELL_DK_FROST_PRESENCE_TRIGGERED)
    target:RemoveAura(SPELL_DK_UNHOLY_PRESENCE_TRIGGERED)
end

for _, spellId in ipairs({ SPELL_DK_BLOOD_PRESENCE, SPELL_DK_FROST_PRESENCE, SPELL_DK_UNHOLY_PRESENCE }) do
    RegisterAuraEvent(spellId, 2, presenceApply)
    RegisterAuraEvent(spellId, 3, presenceRemove)
end

local function improvedBloodPresenceApply(event, aura, target)
    if (target:HasAura(SPELL_DK_FROST_PRESENCE) or target:HasAura(SPELL_DK_UNHOLY_PRESENCE))
        and not target:HasAura(SPELL_DK_IMPROVED_BLOOD_PRESENCE_TRIGGERED) then
        target:CastCustomSpell(target, SPELL_DK_IMPROVED_BLOOD_PRESENCE_TRIGGERED, true, nil, aura:GetAmount())
    end
end

local function improvedBloodPresenceRemove(event, aura, target)
    if not target:HasAura(SPELL_DK_BLOOD_PRESENCE) then
        target:RemoveAura(SPELL_DK_IMPROVED_BLOOD_PRESENCE_TRIGGERED)
    end
end

RegisterAuraEvent(SPELL_DK_IMPROVED_BLOOD_PRESENCE_R1, 2, improvedBloodPresenceApply)
RegisterAuraEvent(SPELL_DK_IMPROVED_BLOOD_PRESENCE_R1, 3, improvedBloodPresenceRemove)

local function improvedFrostPresenceApply(event, aura, target)
    if (target:HasAura(SPELL_DK_BLOOD_PRESENCE) or target:HasAura(SPELL_DK_UNHOLY_PRESENCE))
        and not target:HasAura(SPELL_DK_FROST_PRESENCE_TRIGGERED) then
        target:CastCustomSpell(target, SPELL_DK_FROST_PRESENCE_TRIGGERED, true, aura:GetAmount())
    end
end

local function improvedFrostPresenceRemove(event, aura, target)
    if not target:HasAura(SPELL_DK_FROST_PRESENCE) then
        target:RemoveAura(SPELL_DK_FROST_PRESENCE_TRIGGERED)
    end
end

RegisterAuraEvent(SPELL_DK_IMPROVED_FROST_PRESENCE_R1, 2, improvedFrostPresenceApply)
RegisterAuraEvent(SPELL_DK_IMPROVED_FROST_PRESENCE_R1, 3, improvedFrostPresenceRemove)

local function improvedUnholyPresenceApply(event, aura, target)
    if target:HasAura(SPELL_DK_UNHOLY_PRESENCE)
        and not target:HasAura(SPELL_DK_IMPROVED_UNHOLY_PRESENCE_TRIGGERED) then
        local bp = aura:GetSpellEffectCalcValue(SPELL_DK_IMPROVED_UNHOLY_PRESENCE_R1, 1)
        if bp ~= 0 then
            target:CastCustomSpell(target, SPELL_DK_IMPROVED_UNHOLY_PRESENCE_TRIGGERED, true, bp, bp, bp)
        end
    end
    if (target:HasAura(SPELL_DK_BLOOD_PRESENCE) or target:HasAura(SPELL_DK_FROST_PRESENCE))
        and not target:HasAura(SPELL_DK_UNHOLY_PRESENCE_TRIGGERED) then
        target:CastCustomSpell(target, SPELL_DK_UNHOLY_PRESENCE_TRIGGERED, true, aura:GetAmount())
    end
end

local function improvedUnholyPresenceRemove(event, aura, target)
    target:RemoveAura(SPELL_DK_IMPROVED_UNHOLY_PRESENCE_TRIGGERED)
    if not target:HasAura(SPELL_DK_UNHOLY_PRESENCE) then
        target:RemoveAura(SPELL_DK_UNHOLY_PRESENCE_TRIGGERED)
    end
end

RegisterAuraEvent(SPELL_DK_IMPROVED_UNHOLY_PRESENCE_R1, 2, improvedUnholyPresenceApply)
RegisterAuraEvent(SPELL_DK_IMPROVED_UNHOLY_PRESENCE_R1, 3, improvedUnholyPresenceRemove)

local SPELL_DK_BLOOD_BOIL = 48721 -- spell_dk.cpp:461
local SPELL_DK_BLOOD_BOIL_TRIGGERED = 65658 -- spell_dk.cpp:52
local CLASS_DEATH_KNIGHT = 6

local bloodBoilFired = {}

-- C++ spell_dk_blood_boil_SpellScript::HandleAfterHit
-- (spell_dk.cpp:486-494): AfterHit, once per cast (the _executed
-- latch) on the first live hit unit — the caster self-casts
-- SPELL_DK_BLOOD_BOIL_TRIGGERED 65658 triggered. The Load() gate
-- (caster is a DK player) is the class check below. The ev1
-- (OnCast, fires once per cast before the effect loop, spells.go)
-- reset is the per-cast fresh SpellScript instance; a nil hitTarget
-- is the miss leg (C++ GetHitUnit() nil skips). DORMANT until
-- Spell.dbc ships the 48721/65658 records.
local function bloodBoilReset(event, spell)
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    bloodBoilFired[caster:GetGUIDLow()] = false
end

local function bloodBoilAfterHit(event, spell, hitTarget)
    if hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetClass() ~= CLASS_DEATH_KNIGHT then
        return
    end
    local low = caster:GetGUIDLow()
    if bloodBoilFired[low] then
        return
    end
    bloodBoilFired[low] = true
    caster:CastSpell(caster, SPELL_DK_BLOOD_BOIL_TRIGGERED, true)
end

RegisterSpellEvent(SPELL_DK_BLOOD_BOIL, 1, bloodBoilReset)
RegisterSpellEvent(SPELL_DK_BLOOD_BOIL, 3, bloodBoilAfterHit)

local SPELL_DK_GHOUL_EXPLODE = 47496 -- spell_dk.cpp:1171
local SPELL_DK_CORPSE_EXPLOSION_TRIGGERED = 43999 -- spell_dk.cpp:60
local SPELL_DK_CORPSE_EXPLOSION_VISUAL = 51270 -- spell_dk.cpp:61

-- C++ spell_dk_ghoul_explode_SpellScript::Suicide
-- (spell_dk.cpp:1196-1205): EFFECT_1 OnEffectHitTarget — the hit unit
-- casts SPELL_DK_CORPSE_EXPLOSION_TRIGGERED 43999 triggered on itself,
-- and the caster casts the visual 51270 triggered on the hit unit.
-- The HandleDamage leg (EFFECT_0 OnEffectLaunchTarget, pct of the
-- caster's max health) is the Go spellScriptOnHitDamage 47496 arm
-- (spell_onhit.go); this ev5 fires only in the triggered funnel
-- (castSpellDirectWithOverrides, spells.go) — the living-ghoul leg of
-- corpseExplosionDetonate casts 47496 through it, so the Suicide leg
-- lands there and nowhere else (the Go detonate arm must not also cast
-- 43999 + 51270 for a living ghoul, or it would double-fire).
-- DORMANT until Spell.dbc ships the 47496 record.
local function ghoulExplodeSuicide(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 1 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    hitTarget:CastSpell(hitTarget, SPELL_DK_CORPSE_EXPLOSION_TRIGGERED, true)
    if caster ~= nil then
        caster:CastSpell(hitTarget, SPELL_DK_CORPSE_EXPLOSION_VISUAL, true)
    end
end

RegisterSpellEvent(SPELL_DK_GHOUL_EXPLODE, 5, ghoulExplodeSuicide)

local SPELL_DK_BUTCHERY_R1 = 48979 -- spell_dk.cpp:587
local SPELL_DK_BUTCHERY_R2 = 49483 -- bot_death_knight_ai.cpp:72, rank 2
local SPELL_DK_BUTCHERY_RUNIC_POWER = 50163 -- spell_dk.cpp:88

-- C++ spell_dk_butchery::HandleProc (spell_dk.cpp:603-609): EFFECT_0
-- SPELL_AURA_DUMMY OnEffectProc. PreventDefaultAction (the ev6 true
-- return), then the actor self-casts SPELL_DK_BUTCHERY_RUNIC_POWER
-- 50163 with BP0 = the aura EFFECT_0 amount, triggered via the aurEff
-- overload (CastSpellExtraArgs(aurEff) sets TRIGGERED_FULL_MASK,
-- SpellDefines.h:166) — the CastCustomSpell `true` arg is exact, not
-- a delta. No CheckProc gate, no proc-spell or school filter, so the
-- ev6 args suffice. DORMANT until Spell.dbc ships the 48979/49483
-- proc entries.
local function butcheryProc(event, aura, target, actor, procTarget, damage)
    if actor == nil then
        return true
    end
    actor:CastCustomSpell(actor, SPELL_DK_BUTCHERY_RUNIC_POWER, true, aura:GetAmount())
    return true
end

RegisterAuraEvent(SPELL_DK_BUTCHERY_R1, 6, butcheryProc)
RegisterAuraEvent(SPELL_DK_BUTCHERY_R2, 6, butcheryProc)

local SPELL_DK_DEATH_AND_DECAY_R1 = 43265 -- bot_death_knight_ai.cpp:34
local SPELL_DK_DEATH_AND_DECAY_R2 = 49938 -- PlayerAI.cpp:247
local SPELL_DK_DEATH_AND_DECAY_DAMAGE = 52212 -- spell_dk.cpp:57

-- C++ spell_dk_death_and_decay::HandleDummyTick (spell_dk.cpp:819-827):
-- EFFECT_0 SPELL_AURA_PERIODIC_DUMMY OnEffectPeriodic. The caster
-- casts SPELL_DK_DEATH_AND_DECAY_DAMAGE 52212 on the aura holder with
-- BP0 = the aura EFFECT_0 amount, triggered via the aurEff overload
-- (TRIGGERED_FULL_MASK, SpellDefines.h:166) — the CastCustomSpell
-- `true` arg is exact. Ranks 3-4 (49939/49940) are consecutive IDs
-- with no in-tree citation — not registered until DBC confirms them.
-- DORMANT until Spell.dbc ships the D&D aura records.
local function deathAndDecayTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or target == nil then
        return
    end
    caster:CastCustomSpell(target, SPELL_DK_DEATH_AND_DECAY_DAMAGE, true, aura:GetAmount())
end

RegisterAuraEvent(SPELL_DK_DEATH_AND_DECAY_R1, 1, deathAndDecayTick)
RegisterAuraEvent(SPELL_DK_DEATH_AND_DECAY_R2, 1, deathAndDecayTick)

local SPELL_DK_NECROSIS_R1 = 51459 -- spell_dk.cpp:1735
local SPELL_DK_NECROSIS_DAMAGE = 51460 -- spell_dk.cpp:93

-- C++ spell_dk_necrosis::HandleProc (spell_dk.cpp:1751-1761):
-- EFFECT_0 SPELL_AURA_DUMMY OnEffectProc. PreventDefaultAction (the ev6
-- true return), then the actor casts SPELL_DK_NECROSIS_DAMAGE 51460 on
-- the proc target with BP0 = pct(triggering damage, aura EFFECT_0
-- amount), triggered via the aurEff overload (TRIGGERED_FULL_MASK,
-- SpellDefines.h:166) — the CastCustomSpell `true` arg is exact. No
-- CheckProc gate, no proc-spell or school filter, so the ev6 args
-- (event, aura, target, actor, procTarget, damage) suffice — the
-- player-aura proc path reaches ev6 (procs.go:3921). The damage==0 leg
-- returns after PreventDefaultAction, so it still vetoes.
-- DORMANT until Spell.dbc ships the 51459 proc entry. Higher ranks
-- are not in-tree — not registered until DBC confirms them.
local function necrosisProc(event, aura, target, actor, procTarget, damage)
    if actor == nil or procTarget == nil or damage == nil or damage == 0 then
        return true
    end
    local bp0 = math.floor(damage * aura:GetAmount() / 100)
    actor:CastCustomSpell(procTarget, SPELL_DK_NECROSIS_DAMAGE, true, bp0)
    return true
end

RegisterAuraEvent(SPELL_DK_NECROSIS_R1, 6, necrosisProc)
