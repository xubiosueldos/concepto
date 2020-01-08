package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/xubiosueldos/conexionBD"

	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"github.com/xubiosueldos/autenticacion/apiclientautenticacion"
	"github.com/xubiosueldos/conexionBD/Concepto/structConcepto"
	"github.com/xubiosueldos/framework"
	"github.com/xubiosueldos/monoliticComunication"
)

type IdsAEliminar struct {
	Ids []int `json:"ids"`
}
type aplicaImpuestoGanancias struct {
	Aplicaimpuestosganancias bool `json:"aplicaimpuestosganancias"`
}

/*
func (strhelper) TableName() string {
	return codigoHelper
}*/
var nombreMicroservicio string = "concepto"

// Sirve para controlar si el server esta OK
func Healthy(writer http.ResponseWriter, request *http.Request) {
	writer.Write([]byte("Healthy"))
}

func ConceptoList(w http.ResponseWriter, r *http.Request) {

	tokenValido, tokenAutenticacion := apiclientautenticacion.CheckTokenValido(w, r)
	if tokenValido {

		tenant := apiclientautenticacion.ObtenerTenant(tokenAutenticacion)
		db := conexionBD.ObtenerDB(tenant)

		defer conexionBD.CerrarDB(db)

		var conceptos []structConcepto.Concepto

		//Lista todos los conceptos
		db.Find(&conceptos)
		framework.RespondJSON(w, http.StatusOK, conceptos)
	}

}

/*func crearQueryMixta(concepto string, tenant string) string {
	return "select * from public." + concepto + " as tabla1 where tabla1.deleted_at is null and activo = 1 union all select * from " + tenant + "." + concepto + " as tabla2 where tabla2.deleted_at is null and activo = 1"
}
*/

func ConceptoShow(w http.ResponseWriter, r *http.Request) {

	tokenValido, tokenAutenticacion := apiclientautenticacion.CheckTokenValido(w, r)
	if tokenValido {

		params := mux.Vars(r)
		concepto_id := params["id"]
		p_conceptoid, err := strconv.Atoi(concepto_id)
		if err != nil {
			fmt.Println(err)
		}
		framework.CheckParametroVacio(p_conceptoid, w)
		var concepto structConcepto.Concepto //Con &var --> lo que devuelve el metodo se le asigna a la var

		tenant := apiclientautenticacion.ObtenerTenant(tokenAutenticacion)
		db := conexionBD.ObtenerDB(tenant)
		defer conexionBD.CerrarDB(db)

		//gorm:auto_preload se usa para que complete todos los struct con su informacion
		if err := db.Set("gorm:auto_preload", true).First(&concepto, "id = ?", concepto_id).Error; gorm.IsRecordNotFoundError(err) {
			framework.RespondError(w, http.StatusNotFound, err.Error())
			return
		}

		cuentaID := concepto.CuentaContable
		if cuentaID != nil {
			cuenta := monoliticComunication.Obtenercuenta(w, r, tokenAutenticacion, strconv.Itoa(*cuentaID))
			concepto.Cuentacontable = cuenta
		}

		framework.RespondJSON(w, http.StatusOK, concepto)
	}

}

func ConceptoAdd(w http.ResponseWriter, r *http.Request) {

	tokenValido, tokenAutenticacion := apiclientautenticacion.CheckTokenValido(w, r)
	if tokenValido {

		decoder := json.NewDecoder(r.Body)

		var concepto_data structConcepto.Concepto
		//&concepto_data para decirle que es la var que no tiene datos y va a tener que rellenar
		if err := decoder.Decode(&concepto_data); err != nil {
			framework.RespondError(w, http.StatusBadRequest, err.Error())
			return
		}

		defer r.Body.Close()

		tenant := apiclientautenticacion.ObtenerTenant(tokenAutenticacion)
		db := conexionBD.ObtenerDB(tenant)
		defer conexionBD.CerrarDB(db)
		if canInsertUpdate(w, concepto_data, db) == true {
			if err := monoliticComunication.Checkexistecuenta(w, r, tokenAutenticacion, strconv.Itoa(*concepto_data.CuentaContable)).Error; err != nil {
				framework.RespondError(w, http.StatusInternalServerError, err.Error())
				return
			}

			if concepto_data.Porcentaje != nil && concepto_data.Tipodecalculoid != nil || concepto_data.Porcentaje == nil && concepto_data.Tipodecalculoid == nil {

				if err := db.Create(&concepto_data).Error; err != nil {
					framework.RespondError(w, http.StatusInternalServerError, err.Error())
					return
				}

			} else {
				framework.RespondError(w, http.StatusInternalServerError, "Debe completar el Porcentaje o el Cálculo entre Conceptos")
				return
			}

			framework.RespondJSON(w, http.StatusCreated, concepto_data)
		}
	}
}

func ConceptoUpdate(w http.ResponseWriter, r *http.Request) {

	tokenValido, tokenAutenticacion := apiclientautenticacion.CheckTokenValido(w, r)
	if tokenValido {

		params := mux.Vars(r)
		//se convirtió el string en uint para poder comparar
		p_conpcetoid, err := strconv.Atoi(params["id"])
		if err != nil {
			fmt.Println(err)
		}

		framework.CheckParametroVacio(p_conpcetoid, w)
		framework.CheckRegistroDefault(p_conpcetoid, w)
		decoder := json.NewDecoder(r.Body)

		var concepto_data structConcepto.Concepto

		if err := decoder.Decode(&concepto_data); err != nil {
			framework.RespondError(w, http.StatusBadRequest, err.Error())
			return
		}
		defer r.Body.Close()

		conpcetoid := concepto_data.ID

		if err := monoliticComunication.Checkexistecuenta(w, r, tokenAutenticacion, strconv.Itoa(*concepto_data.CuentaContable)).Error; err != nil {
			framework.RespondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		if p_conpcetoid == conpcetoid || conpcetoid == 0 {

			concepto_data.ID = p_conpcetoid

			tenant := apiclientautenticacion.ObtenerTenant(tokenAutenticacion)
			db := conexionBD.ObtenerDB(tenant)
			defer conexionBD.CerrarDB(db)

			if concepto_data.Porcentaje != nil && concepto_data.Tipodecalculoid != nil || concepto_data.Porcentaje == nil && concepto_data.Tipodecalculoid == nil {

				//abro una transacción para que si hay un error no persista en la DB
				tx := db.Begin()

				//modifico el concepto de acuerdo a lo enviado en el json
				if err := tx.Save(&concepto_data).Error; err != nil {
					tx.Rollback()
					framework.RespondError(w, http.StatusInternalServerError, err.Error())
					return
				}

				tx.Commit()

				framework.RespondJSON(w, http.StatusOK, concepto_data)

			} else {
				framework.RespondError(w, http.StatusInternalServerError, "Debe completar el Porcentaje o el Cálculo entre Conceptos")
				return

			}

		} else {
			framework.RespondError(w, http.StatusNotFound, framework.IdParametroDistintoStruct)
			return
		}
	}

}

func ConceptoRemove(w http.ResponseWriter, r *http.Request) {

	tokenValido, tokenAutenticacion := apiclientautenticacion.CheckTokenValido(w, r)
	if tokenValido {

		//Para obtener los parametros por la url
		params := mux.Vars(r)
		p_conpcetoid, err := strconv.Atoi(params["id"])
		if err != nil {
			fmt.Println(err)
		}

		tenant := apiclientautenticacion.ObtenerTenant(tokenAutenticacion)
		db := conexionBD.ObtenerDB(tenant)
		defer conexionBD.CerrarDB(db)
		/*
			var conceptos structConcepto.Concepto //Con &var --> lo que devuelve el metodo se le asigna a la var

				if err := db.Set("gorm:auto_preload", true).Raw(" select * from (" + crearQueryMixta("concepto", tokenAutenticacion.Tenant) + ") as tabla where tabla.id = " + concepto_id).Scan(&conceptos).Error; gorm.IsRecordNotFoundError(err) {
					framework.RespondError(w, http.StatusNotFound, err.Error())
					return
				}

				var requestMono requestMono

				if err := requestMono.requestMonolitico("CANQUERY", w, r, conceptos, tokenAutenticacion, "cuenta").Error; err != nil {
					framework.RespondError(w, http.StatusInternalServerError, err.Error())
					return
				}*/

		//--Borrado Fisico
		framework.CheckParametroVacio(p_conpcetoid, w)
		framework.CheckRegistroDefault(p_conpcetoid, w)
		if err := db.Unscoped().Where("id = ?", p_conpcetoid).Delete(structConcepto.Concepto{}).Error; err != nil {

			framework.RespondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		framework.RespondJSON(w, http.StatusOK, framework.Concepto+strconv.Itoa(p_conpcetoid)+framework.MicroservicioEliminado)
	}

}

func ConceptosRemoveMasivo(w http.ResponseWriter, r *http.Request) {
	var resultadoDeEliminacion = make(map[int]string)
	tokenValido, tokenAutenticacion := apiclientautenticacion.CheckTokenValido(w, r)
	if tokenValido {

		var idsEliminar IdsAEliminar
		decoder := json.NewDecoder(r.Body)

		if err := decoder.Decode(&idsEliminar); err != nil {
			framework.RespondError(w, http.StatusBadRequest, err.Error())
			return
		}

		tenant := apiclientautenticacion.ObtenerTenant(tokenAutenticacion)
		db := conexionBD.ObtenerDB(tenant)

		defer conexionBD.CerrarDB(db)

		if len(idsEliminar.Ids) > 0 {
			for i := 0; i < len(idsEliminar.Ids); i++ {
				concepto_id := idsEliminar.Ids[i]
				if err := db.Unscoped().Where("id = ?", concepto_id).Delete(structConcepto.Concepto{}).Error; err != nil {
					//framework.RespondError(w, http.StatusInternalServerError, err.Error())
					resultadoDeEliminacion[concepto_id] = string(err.Error())

				} else {
					resultadoDeEliminacion[concepto_id] = "Fue eliminado con exito"
				}
			}
		} else {
			framework.RespondError(w, http.StatusInternalServerError, "Seleccione por lo menos un registro")
		}

		framework.RespondJSON(w, http.StatusOK, resultadoDeEliminacion)
	}

}

type prueba struct {
	aplica bool
}

func canInsertUpdate(w http.ResponseWriter, concepto structConcepto.Concepto, db *gorm.DB) bool {
	if concepto.Tipoimpuestogananciasid != nil {
		var aplicaimpuestoganancias prueba
		var tipoConcepto structConcepto.Tipoconcepto

		sql := "SELECT CODIGO FROM TipoConcepto WHERE ID = " + strconv.Itoa(*concepto.Tipoconceptoid)
		db.Set("gorm:auto_preload", true).Raw(sql).Scan(&tipoConcepto)

		/*aplicaTipoConcepto := "APLICA" + strings.ReplaceAll(tipoConcepto.Codigo, "_", "")

		db.Table("TIPOIMPUESTOGANANCIAS").Select(aplicaTipoConcepto).Find(&aplicaimpuestoganancias)*/
		sql = "SELECT APLICA" + strings.ReplaceAll(tipoConcepto.Codigo, "_", "") + " AS APLICA FROM TIPOIMPUESTOGANANCIAS WHERE ID = " + strconv.Itoa(*concepto.Tipoimpuestogananciasid)
		fmt.Println(sql)
		db.Raw(sql).Scan(&aplicaimpuestoganancias)

		if aplicaimpuestoganancias.aplica == true {
			if *concepto.Tipoconceptoid == -4 {
				if concepto.Prorrateo == true && concepto.Basesac == true {
					framework.RespondError(w, http.StatusInternalServerError, "No se puede seleccionar Prorrateo y Base Sac porque el Concepto es de tipo Retencion")
					return false

				}
			}
		} else {

		}
	}
	return true
}
