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
--   (49016, ev1 self-damage) and spell_dk_death_grip_initial
--   (49576, ev8 + ev5) are PORTED below — the tick-294
--   "hysteria needs DealDamage" stay-out is stale, the bridge
--   exists. spell_dk_raise_ally needs ghoul AI push/pop.
--   spell_dk_death_coil ranks 1-4 (47541/52375/59134/62900): CheckCast
--   arm PORTED this tick (OnCheckCast ev8 below); the Go
--   finishSpellCast dummy leg stays PORTED (spells.go deathCoilDummyEffect
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
--   player cast helper — the 47496 ghoul precedent). spell_dk_
--   pestilence (50842, tick 567) stays out — no ev5 player path, and
--   no aura donePct/critChance/ChangeAmount bridges for the disease
--   copy legs. spell_dk_rune_tap_party (59754, tick 569) stays out —
--   the OnObjectAreaTargetSelect fire site is creature-path only, no
--   player-path twin. Rime (49188), scent_of_blood_trigger (49004)
--   and sudden_doom (49018) are PORTED below (ev6, ticks 568/570/571).
--   Vendetta (49015) is PORTED below (ev6, tick 573). Threat of
--   Thassarian (65661, tick 572) stays out — the ev6 args carry no
--   proc spell id/family flags (dancing_rune_weapon precedent,
--   procs.go procAuraTriggerLoopFiltered), no GetSpellWithRank
--   bridge, and the offhand gate is only approximable via
--   GetEquippedItemBySlot(16). Wandering Plague (49217, tick 574)
--   stays out — no GetUnitCriticalChanceAgainst bridge anywhere in
--   the engine; the crit-chance roll is the whole gate.
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
--   the EFFECT_0 value (the teleport spell); CheckClass PORTED this
--   tick (OnCheckCast ev8 below; the custom-error message text has no
--   Go twin — the fire site sends only the failure code); the player
--   leg rides the triggered funnel where C++ casts plain (documented
--   delta). DORMANT until Spell.dbc ships 52751.
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
--   spell_dk_death_gate (52751, 2026-11-10): PORTED in Go, not Lua — the
--   handleEffectScriptEffect case (spells.go) + the CheckClass gate
--   PORTED this tick (OnCheckCast ev8 below); a Lua ev5 port would never
--   fire (the player path has no OnEffectHitTarget site, 60533 precedent).
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

local SPELL_DK_RIME = 49188 -- spell_dk.cpp:2127
local SPELL_CATEGORY_HOWLING_BLAST = 1248 -- spell_dk.cpp:116

-- C++ spell_dk_rime::HandleProc (spell_dk.cpp:2142-2149): EFFECT_1
-- SPELL_AURA_PROC_TRIGGER_SPELL OnEffectProc. The target's spell
-- history resets cooldowns whose DBC category is
-- SPELL_CATEGORY_HOWLING_BLAST (1248), update=true. The DoCheckProc
-- target-is-player gate rides the handler (the creature ev6 loop at
-- boss_ai.go:10884 can pass a non-player target, and
-- ResetTypeCooldowns is a player-only method). No
-- PreventDefaultAction: the DBC default trigger still fires after
-- (the handler returns nothing). DORMANT until Spell.dbc ships the
-- 49188 proc entry.
local function rimeProc(event, aura, target, actor, procTarget, damage)
    if target == nil or target:GetTypeId() ~= 4 then
        return
    end
    target:ResetTypeCooldowns(SPELL_CATEGORY_HOWLING_BLAST, true)
end

RegisterAuraEvent(SPELL_DK_RIME, 6, rimeProc)

local SPELL_DK_SCENT_OF_BLOOD_TRIGGER = 49004 -- spell_dk.cpp:2225

-- C++ spell_dk_scent_of_blood_trigger::HandleProc (spell_dk.cpp:2237-2240):
-- EFFECT_0 SPELL_AURA_PROC_TRIGGER_SPELL OnEffectProc. Removes the old
-- buff (the effect's DBC trigger spell) from the holder so the re-proc
-- does not stack onto an existing aura. No PreventDefaultAction: the
-- DBC default trigger still fires after (the handler returns nothing).
-- Distinct from the talent aura 50421, whose Go leg lives in procs.go.
-- DORMANT until Spell.dbc ships the 49004 proc entry.
local function scentOfBloodTriggerProc(event, aura, target, actor, procTarget, damage)
    if aura == nil or target == nil then
        return
    end
    local trig = aura:GetTriggerSpell(0)
    if trig ~= 0 then
        target:RemoveAura(trig)
    end
end

RegisterAuraEvent(SPELL_DK_SCENT_OF_BLOOD_TRIGGER, 6, scentOfBloodTriggerProc)

local SPELL_DK_SUDDEN_DOOM = 49018 -- spell_dk.cpp:2382
local SUDDEN_DOOM_DEATH_COIL_RANKS = {47541, 52375, 59134, 62900} -- spell_dk.cpp:840

-- C++ spell_dk_sudden_doom::HandleProc (spell_dk.cpp:2398-2422):
-- EFFECT_0 SPELL_AURA_DUMMY OnEffectProc. PreventDefaultAction first
-- (the ev6 true return on every path, even the no-rank path), then the
-- actor casts the highest known Death Coil rank on the proc target,
-- triggered via the aurEff overload (the player CastSpell funnel).
-- The rank walk is GetNextRankSpell + HasSpell in C++; no
-- GetNextRankSpell bridge exists, so the walk is the hardcoded
-- ascending rank list from the death-coil class (spell_dk.cpp:840).
-- DORMANT until Spell.dbc ships the 49018 proc entry.
local function suddenDoomProc(event, aura, target, actor, procTarget, damage)
    if actor == nil or procTarget == nil then
        return true
    end
    local known = 0
    for _, rank in ipairs(SUDDEN_DOOM_DEATH_COIL_RANKS) do
        if actor:HasSpell(rank) then
            known = rank
        else
            break
        end
    end
    if known == 0 then
        return true
    end
    actor:CastSpell(procTarget, known)
    return true
end

RegisterAuraEvent(SPELL_DK_SUDDEN_DOOM, 6, suddenDoomProc)

local SPELL_DK_VENDETTA = 49015 -- spell_dk.cpp:2590
local SPELL_DK_VENDETTA_HEAL = 50181 -- spell_dk.cpp:92

-- C++ spell_dk_vendetta::HandleProc (spell_dk.cpp:2605-2612):
-- EFFECT_0 SPELL_AURA_DUMMY OnEffectProc. PreventDefaultAction first
-- (the ev6 true return on every path), then the actor self-casts the
-- heal 50181 with BP0 = CountPctFromMaxHealth(this aura's EFFECT_0
-- amount), triggered via the aurEff overload. The nil CastCustomSpell
-- target falls back to the caster (lua_player.go CastCustomSpell).
-- DORMANT until Spell.dbc ships the 49015 proc entry. Rank 1 only —
-- higher-rank IDs are not in the file, never guessed.
local function vendettaProc(event, aura, target, actor, procTarget, damage)
    if actor == nil or aura == nil then
        return true
    end
    local bp0 = actor:CountPctFromMaxHealth(aura:GetAmount())
    actor:CastCustomSpell(nil, SPELL_DK_VENDETTA_HEAL, true, bp0)
    return true
end

RegisterAuraEvent(SPELL_DK_VENDETTA, 6, vendettaProc)

local SPELL_DK_BLADE_BARRIER = 49182 -- spell_dk.cpp:429 (class spell_dk_blade_barrier)
local CLASS_DEATH_KNIGHT = 6 -- SharedDefines.h:132
local RUNE_BLOOD = 0 -- Player.h:269

-- C++ spell_dk_blade_barrier::CheckProc (spell_dk.cpp:440-450):
-- DoCheckProc only, no HandleProc — the DBC default trigger leg
-- fires when the check passes. Passes only when the proc event
-- carries a spell (GetSpellInfo() != nil; a nil event spell reads
-- 0 via procEvent:GetSpellId) and the actor is a death knight
-- player whose base blood rune slots are all on cooldown
-- (IsBaseRuneSlotsOnCooldown, lua_player.go). A failed check
-- vetoes (return true); a pass returns nothing so the default leg
-- fires, the rime precedent. DORMANT until Spell.dbc ships the
-- 49182 proc entry.
local function bladeBarrierProc(event, aura, target, actor, procTarget, damage, procEvent)
    if procEvent == nil or procEvent:GetSpellId() == 0 then
        return true
    end
    if actor == nil or actor:GetTypeId() ~= 4 or actor:GetClass() ~= CLASS_DEATH_KNIGHT then
        return true
    end
    if not actor:IsBaseRuneSlotsOnCooldown(RUNE_BLOOD) then
        return true
    end
end

RegisterAuraEvent(SPELL_DK_BLADE_BARRIER, 6, bladeBarrierProc)

local SPELL_DK_ACCLIMATION = 49200 -- spell_dk.cpp:120 (rank 1; ranks 2-3 IDs are not in the file, never guessed)
local SPELL_DK_ACCLIMATION_HOLY = 50490 -- spell_dk.cpp:43
local SPELL_DK_ACCLIMATION_FIRE = 50362 -- spell_dk.cpp:44
local SPELL_DK_ACCLIMATION_NATURE = 50488 -- spell_dk.cpp:48
local SPELL_DK_ACCLIMATION_FROST = 50485 -- spell_dk.cpp:45
local SPELL_DK_ACCLIMATION_SHADOW = 50489 -- spell_dk.cpp:47
local SPELL_DK_ACCLIMATION_ARCANE = 50486 -- spell_dk.cpp:46
local SPELL_SCHOOL_MASK_HOLY = 2 -- SharedDefines.h:325
local SPELL_SCHOOL_MASK_FIRE = 4 -- SharedDefines.h:326
local SPELL_SCHOOL_MASK_NATURE = 8 -- SharedDefines.h:327
local SPELL_SCHOOL_MASK_FROST = 16 -- SharedDefines.h:328
local SPELL_SCHOOL_MASK_SHADOW = 32 -- SharedDefines.h:329
local SPELL_SCHOOL_MASK_ARCANE = 64 -- SharedDefines.h:330

-- C++ spell_dk_acclimation::CheckProc + HandleProc (spell_dk.cpp:144-192):
-- EFFECT_0 SPELL_AURA_PROC_TRIGGER_SPELL OnEffectProc. CheckProc passes
-- only when the damage info exists and its first school is a magic
-- school (GetFirstSchoolInMask); HandleProc PreventDefaultActions
-- (the true return on every path) and casts the school's trigger
-- spell from the action target onto itself. The school and action
-- target ride the forwarded proc event (procEventObject, lua_aura.go:
-- GetSchoolMask / GetActionTarget); a nil proc event or a non-magic
-- school vetoes (true) so the DBC default leg never fires on a failed
-- gate, matching C++ where a failed CheckProc procs nothing. DORMANT
-- until Spell.dbc ships the 49200 proc entry.
local function acclimationProc(event, aura, target, actor, procTarget, damage, procEvent)
    if procEvent == nil then
        return true
    end
    local schoolMask = procEvent:GetSchoolMask()
    local triggerSpell = 0
    if bit32.band(schoolMask, SPELL_SCHOOL_MASK_HOLY) ~= 0 then
        triggerSpell = SPELL_DK_ACCLIMATION_HOLY
    elseif bit32.band(schoolMask, SPELL_SCHOOL_MASK_FIRE) ~= 0 then
        triggerSpell = SPELL_DK_ACCLIMATION_FIRE
    elseif bit32.band(schoolMask, SPELL_SCHOOL_MASK_NATURE) ~= 0 then
        triggerSpell = SPELL_DK_ACCLIMATION_NATURE
    elseif bit32.band(schoolMask, SPELL_SCHOOL_MASK_FROST) ~= 0 then
        triggerSpell = SPELL_DK_ACCLIMATION_FROST
    elseif bit32.band(schoolMask, SPELL_SCHOOL_MASK_SHADOW) ~= 0 then
        triggerSpell = SPELL_DK_ACCLIMATION_SHADOW
    elseif bit32.band(schoolMask, SPELL_SCHOOL_MASK_ARCANE) ~= 0 then
        triggerSpell = SPELL_DK_ACCLIMATION_ARCANE
    end
    if triggerSpell == 0 then
        return true
    end
    local actionTarget = procEvent:GetActionTarget()
    if actionTarget == nil then
        return true
    end
    actionTarget:CastSpell(actionTarget, triggerSpell, true)
    return true
end

RegisterAuraEvent(SPELL_DK_ACCLIMATION, 6, acclimationProc)

local SPELL_FAILED_BAD_TARGETS = 12 -- SharedDefines.h:994
local SPELL_FAILED_MOVING = 51 -- SharedDefines.h:1033
local SPELL_FAILED_TOO_CLOSE = 128 -- SharedDefines.h:1110
local SPELL_FAILED_UNIT_NOT_INFRONT = 134 -- SharedDefines.h:1116
local SPELL_FAILED_CUSTOM_ERROR = 172 -- SharedDefines.h:1154
local SPELL_CAST_OK = 255 -- SharedDefines.h:1171 (custom value, never sent to the client)
local CREATURE_TYPE_UNDEAD = 6 -- SharedDefines.h:2666

-- C++ spell_dk_death_coil_SpellScript::CheckCast (spell_dk.cpp:878-893):
-- no explicit target fails BAD_TARGETS; a hostile target out of the
-- frontal arc fails UNIT_NOT_INFRONT; a target friendly to the caster
-- that is not undead fails BAD_TARGETS. Check-only port: the dummy
-- damage/heal legs stay in Go (spells.go deathCoilDummyEffect).
-- RegisterSpellEvent OnCheckCast = 8 (lua_spell.go fireSpellCheckCast).
local function deathCoilCheckCast(event, spell)
    local caster = spell:GetCaster()
    local target = spell:GetTarget()
    if target == nil then
        return SPELL_FAILED_BAD_TARGETS
    end
    if caster ~= nil and not caster:IsFriendlyTo(target) and not caster:IsInFront(target) then
        return SPELL_FAILED_UNIT_NOT_INFRONT
    end
    if target:IsFriendlyTo(caster) and target:GetCreatureType() ~= CREATURE_TYPE_UNDEAD then
        return SPELL_FAILED_BAD_TARGETS
    end
    return SPELL_CAST_OK
end

for _, spellId in ipairs(SUDDEN_DOOM_DEATH_COIL_RANKS) do
    RegisterSpellEvent(spellId, 8, deathCoilCheckCast)
end

local SPELL_DK_DEATH_GATE = 52751 -- spell_dk.cpp:909 (class spell_dk_death_gate_SpellScript)

-- C++ spell_dk_death_gate_SpellScript::CheckClass (spell_dk.cpp:918-927):
-- a non-Death-Knight caster fails SPELL_FAILED_CUSTOM_ERROR (C++ also
-- sets the MUST_BE_DEATH_KNIGHT custom message; the Go fire site sends
-- only the failure code, so the message text has no twin). RegisterSpellEvent
-- OnCheckCast = 8 (lua_spell.go fireSpellCheckCast).
local function deathGateCheckCast(event, spell)
    local caster = spell:GetCaster()
    if caster ~= nil and caster:GetClass() ~= CLASS_DEATH_KNIGHT then
        return SPELL_FAILED_CUSTOM_ERROR
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_DK_DEATH_GATE, 8, deathGateCheckCast)

local SPELL_DK_HYSTERIA = 49016 -- spell_dk.cpp:1327 (class spell_dk_hysteria)

-- C++ spell_dk_hysteria::PeriodicTick (spell_dk.cpp:1327-1342):
-- EFFECT_1 PERIODIC_DUMMY tick deals CountPctFromMaxHealth of the
-- effect-1 amount as self damage (SELF_DAMAGE, school NORMAL, no
-- durability loss). RegisterAuraEvent OnEffectPeriodic = 1. The
-- tick-294 "needs DealDamage" stay-out is stale: DealDamage is
-- bridged on player + creature + objects (lua_player.go:3460,
-- lua_creature_events.go:1874, lua_objects.go:2585) with the
-- algalon self-damage precedent, and GetAmount(1) reads the
-- EFFECT_1 live amount (lua_aura.go:418).
local function hysteriaTick(event, aura, target)
    if target == nil then
        return
    end
    local damage = target:CountPctFromMaxHealth(aura:GetAmount(1))
    target:DealDamage(target, damage, false, 0)
end

RegisterAuraEvent(SPELL_DK_HYSTERIA, 1, hysteriaTick)

local SPELL_DK_DEATH_GRIP_INITIAL = 49576 -- spell_dk.cpp:2740
local SPELL_DK_DEATH_GRIP = 49560 -- spell_dk.cpp:60
local PLAYER_COMBAT_REACH = 1.5 -- ObjectDefines.h:40 DEFAULT_PLAYER_COMBAT_REACH

-- C++ spell_dk_death_grip_initial (spell_dk.cpp:2740-2781):
-- CheckCast (OnCheckCast = 8, lua_spell.go fireSpellCheckCast):
-- jumping or falling fails SPELL_FAILED_MOVING; a player target
-- closer than 8 yards fails SPELL_FAILED_TOO_CLOSE. The jumping
-- leg is unported, not ported dead: HasUnitState(JUMPING) has no
-- live model, it reads the script mask only (lua_unit_state.go),
-- so it could never fire; the falling leg rides the player-only
-- IsFalling session flag (lua_player.go:838), gated on the caster
-- being a player. The 8-yard leg is C++-exact:
-- WorldObject::GetDistance subtracts both combat reaches
-- (Object.cpp:1169-1173) while the Go twin is raw 3D, so the
-- player reaches come off as the 1.5 constant (maiden_of_virtue
-- precedent; GetCombatReach is creature-surface only).
-- HandleDummy (OnEffectHitTarget = 5, EFFECT_0 DUMMY): the caster
-- casts Death Grip 49560 on the hit unit, triggered.
local function deathGripInitialCheckCast(event, spell)
    local caster = spell:GetCaster()
    if caster == nil then
        return SPELL_CAST_OK
    end
    if caster:GetObjectType() == "Player" and caster:IsFalling() then
        return SPELL_FAILED_MOVING
    end
    local target = spell:GetTarget()
    if target ~= nil and target:GetObjectType() == "Player" then
        local dist = caster:GetDistance(target) - PLAYER_COMBAT_REACH - PLAYER_COMBAT_REACH
        if dist < 0 then
            dist = 0
        end
        if dist < 8 then
            return SPELL_FAILED_TOO_CLOSE
        end
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_DK_DEATH_GRIP_INITIAL, 8, deathGripInitialCheckCast)

local function deathGripInitialHit(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(target, SPELL_DK_DEATH_GRIP, true)
end

RegisterSpellEvent(SPELL_DK_DEATH_GRIP_INITIAL, 5, deathGripInitialHit)

-- PORTED (this tick, RegisterAuraEvent OnDispel = 5):
--   spell_dk_frost_fever (55095, spell_dk.cpp:1343-1358): AfterDispel.
--   When Frost Fever is dispelled, if its caster exists and the owner
--   still carries an Icy Clutch slow (SPELL_AURA_MOD_DECREASE_SPEED,
--   SPELLFAMILY_DEATHKNIGHT, flag 0x00040000) from that caster, that
--   aura is removed by spell id. Rides the Go AuraEventOnDispel bridge
--   (lua_aura.go fireAuraDispel; hexlord ev5 precedent) and the
--   GetAuraEffectSpellIdByFamily player+creature twin (hunter
--   thrill-of-the-hunt precedent); RemoveAura(id) is the
--   RemoveAurasDueToSpell twin.
local SPELL_DK_FROST_FEVER = 55095 -- spell_dk.cpp:1343 (class), :3235 (RegisterSpellScript)
local SPELL_AURA_MOD_DECREASE_SPEED = 33 -- SpellAuraDefines.h:113
local SPELLFAMILY_DEATHKNIGHT = 15 -- SharedDefines.h:3596
local SPELL_FAMILY_FLAG_DK_ICY_CLUTCH = 0x00040000 -- spell_dk.cpp:1350

local function frostFeverDispel(event, aura, target, dispeller)
    local caster = aura:GetCaster()
    if caster == nil or target == nil then
        return
    end
    local icyClutch = target:GetAuraEffectSpellIdByFamily(SPELL_AURA_MOD_DECREASE_SPEED, SPELLFAMILY_DEATHKNIGHT, 0, SPELL_FAMILY_FLAG_DK_ICY_CLUTCH, 0, caster:GetGUID())
    if icyClutch ~= nil then
        target:RemoveAura(icyClutch)
    end
end

RegisterAuraEvent(SPELL_DK_FROST_FEVER, 5, frostFeverDispel)
